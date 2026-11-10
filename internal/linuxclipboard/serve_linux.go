package linuxclipboard

import (
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type transferKey struct {
	window   xproto.Window
	property xproto.Atom
}
type outgoing struct {
	target            xproto.Atom
	data              []byte
	offset            int
	deadline, expires time.Time
}

func (x *xClipboard) serve(e xproto.SelectionRequestEvent) {
	property := e.Property
	if property == 0 {
		property = e.Target
	} // ICCCM compatibility with old clients.
	notify := xproto.SelectionNotifyEvent{Time: e.Time, Requestor: e.Requestor, Selection: e.Selection, Target: e.Target}
	if e.Owner != x.window || x.owner != x.window || e.Selection != x.a.clipboard || x.value.ID == "" {
		x.sendNotify(notify)
		return
	}
	var data []byte
	target, format := e.Target, byte(8)
	switch e.Target {
	case x.a.targets:
		target, format = xproto.AtomAtom, 32
		targets := []xproto.Atom{x.a.targets}
		if x.value.Format == "png" {
			targets = append(targets, x.a.png)
		} else {
			targets = append(targets, x.a.utf8, x.a.plain, x.a.text)
			if _, ok := textLatin1(x.value.Text); ok {
				targets = append(targets, xproto.AtomString)
			}
		}
		data = make([]byte, len(targets)*4)
		for i, atom := range targets {
			xgb.Put32(data[i*4:], uint32(atom))
		}
	case x.a.png:
		if x.value.Format != "png" {
			x.sendNotify(notify)
			return
		}
		data = x.value.Data
	case x.a.utf8, x.a.plain, x.a.text:
		if x.value.Format != "" {
			x.sendNotify(notify)
			return
		}
		data = []byte(x.value.Text)
		if e.Target == x.a.text {
			target = x.a.utf8
		}
	case xproto.AtomString:
		if x.value.Format != "" {
			x.sendNotify(notify)
			return
		}
		var ok bool
		data, ok = textLatin1(x.value.Text)
		if !ok {
			x.sendNotify(notify)
			return
		}
	default:
		x.sendNotify(notify)
		return
	}
	key := transferKey{e.Requestor, property}
	if _, busy := x.transfers[key]; busy {
		x.sendNotify(notify)
		return
	}
	if len(data) <= x.chunk {
		if err := xproto.ChangePropertyChecked(x.c, xproto.PropModeReplace, e.Requestor, property, target, format, uint32(len(data)/(int(format)/8)), data).Check(); err == nil {
			notify.Property = property
		}
	} else if len(x.transfers) < 8 {
		mask := uint32(xproto.EventMaskPropertyChange | xproto.EventMaskStructureNotify)
		if err := xproto.ChangeWindowAttributesChecked(x.c, e.Requestor, xproto.CwEventMask, []uint32{mask}).Check(); err == nil {
			size := make([]byte, 4)
			xgb.Put32(size, uint32(len(data)))
			if err := xproto.ChangePropertyChecked(x.c, xproto.PropModeReplace, e.Requestor, property, x.a.incr, 32, 1, size).Check(); err == nil {
				now := time.Now()
				x.transfers[key] = &outgoing{target: target, data: data, deadline: now.Add(5 * time.Second), expires: now.Add(30 * time.Second)}
				notify.Property = property
			} else {
				x.unwatch(e.Requestor)
			}
		}
	}
	x.sendNotify(notify)
}

func (x *xClipboard) sendNotify(e xproto.SelectionNotifyEvent) {
	_ = xproto.SendEventChecked(x.c, false, e.Requestor, xproto.EventMaskNoEvent, string(e.Bytes())).Check()
}

func (x *xClipboard) advance(e xproto.PropertyNotifyEvent) {
	if e.State != xproto.PropertyDelete {
		return
	}
	key := transferKey{e.Window, e.Atom}
	t := x.transfers[key]
	if t == nil {
		return
	}
	now := time.Now()
	if !now.Before(t.deadline) || !now.Before(t.expires) {
		x.dropTransfer(key)
		return
	}
	end := min(t.offset+x.chunk, len(t.data))
	chunk := t.data[t.offset:end]
	err := xproto.ChangePropertyChecked(x.c, xproto.PropModeReplace, e.Window, e.Atom, t.target, 8, uint32(len(chunk)), chunk).Check()
	if err != nil || len(chunk) == 0 {
		x.dropTransfer(key)
		return
	}
	t.offset, t.deadline = end, now.Add(5*time.Second)
}

func (x *xClipboard) expireTransfers(now time.Time) {
	for key, t := range x.transfers {
		if !now.Before(t.deadline) || !now.Before(t.expires) {
			x.dropTransfer(key)
		}
	}
}
func (x *xClipboard) dropTransfer(key transferKey) { delete(x.transfers, key); x.unwatch(key.window) }
func (x *xClipboard) dropWindow(w xproto.Window) {
	for key := range x.transfers {
		if key.window == w {
			delete(x.transfers, key)
		}
	}
}
func (x *xClipboard) unwatch(w xproto.Window) {
	for key := range x.transfers {
		if key.window == w {
			return
		}
	}
	xproto.ChangeWindowAttributes(x.c, w, xproto.CwEventMask, []uint32{xproto.EventMaskNoEvent})
}
