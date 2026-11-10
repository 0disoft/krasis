package linuxclipboard

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func (x *xClipboard) readSelection(ctx context.Context, revision uint64) (peerupdate.ClipboardValue, error) {
	var value peerupdate.ClipboardValue
	current, err := x.current()
	if err != nil {
		return value, err
	}
	if current.revision != revision {
		return value, ErrChanged
	}
	if current.owned || x.owner == 0 {
		return value, ErrUnsupported
	}
	w, err := x.newWindow()
	if err != nil {
		return value, err
	}
	// A new requestor for each read isolates delayed SelectionNotify replies.
	// Destroying it aborts an oversized, canceled or stalled INCR and releases
	// the owner from waiting for further property deletes.
	defer func() { _ = xproto.DestroyWindowChecked(x.c, w).Check() }()
	targets, err := x.convert(ctx, w, x.a.targets, 16<<10, revision)
	target := xproto.Atom(0)
	if err == nil {
		if targets.Type != xproto.AtomAtom || targets.Format != 32 || len(targets.Value)%4 != 0 {
			return value, ErrUnsupported
		}
		available := make(map[xproto.Atom]bool)
		for i := 0; i < len(targets.Value); i += 4 {
			available[xproto.Atom(xgb.Get32(targets.Value[i:]))] = true
		}
		for _, candidate := range []xproto.Atom{x.a.png, x.a.utf8, x.a.plain, x.a.text, xproto.AtomString} {
			if available[candidate] {
				target = candidate
				break
			}
		}
		if target == 0 {
			return value, ErrUnsupported
		}
	} else if err == ErrUnsupported {
		// Old owners may omit TARGETS but still implement UTF8_STRING.
		target = x.a.utf8
	} else {
		return value, err
	}
	limit := peerupdate.MaxClipboardBytes
	if target == x.a.png {
		limit = peerupdate.MaxClipboardImageBytes
	}
	property, err := x.convert(ctx, w, target, limit, revision)
	if err != nil {
		return value, err
	}
	if property.Format != 8 {
		return value, ErrUnsupported
	}
	if target == x.a.png {
		if property.Type != x.a.png {
			return value, ErrUnsupported
		}
		value.Format, value.Data = "png", property.Value
	} else {
		if property.Type == xproto.AtomString {
			if target != xproto.AtomString && target != x.a.text {
				return value, ErrUnsupported
			}
			value.Text = latin1Text(property.Value)
		} else {
			if property.Type != x.a.utf8 && property.Type != x.a.plain {
				return value, ErrUnsupported
			}
			value.Text = string(property.Value)
		}
	}
	if err = validateValue(value); err != nil {
		return peerupdate.ClipboardValue{}, err
	}
	if err = ctx.Err(); err != nil {
		return peerupdate.ClipboardValue{}, err
	}
	current, err = x.current()
	if err != nil {
		return peerupdate.ClipboardValue{}, err
	}
	if current.revision != revision {
		return peerupdate.ClipboardValue{}, ErrChanged
	}
	return value, nil
}

func (x *xClipboard) convert(ctx context.Context, w xproto.Window, target xproto.Atom, limit int, revision uint64) (*xproto.GetPropertyReply, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := xproto.DeletePropertyChecked(x.c, w, x.a.property).Check(); err != nil {
		return nil, err
	}
	if err := xproto.ConvertSelectionChecked(x.c, w, x.a.clipboard, target, x.a.property, xproto.TimeCurrentTime).Check(); err != nil {
		return nil, err
	}
	timeout := time.NewTimer(750 * time.Millisecond)
	defer timeout.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var reply *xproto.SelectionNotifyEvent
		x.drain(func(e xproto.SelectionNotifyEvent) {
			if e.Requestor == w && e.Selection == x.a.clipboard && e.Target == target {
				reply = &e
			}
		})
		if x.revision != revision {
			return nil, ErrChanged
		}
		if reply != nil {
			if reply.Property == 0 {
				return nil, ErrUnsupported
			}
			if reply.Property != x.a.property {
				return nil, ErrUnsupported
			}
			// GetProperty length is in four-byte units. BytesAfter must be zero
			// before accepting data; truncation is never mistaken for success.
			p, err := xproto.GetProperty(x.c, false, w, x.a.property, xproto.GetPropertyTypeAny, 0, uint32((limit+3)/4)).Reply()
			if err != nil {
				return nil, err
			}
			if p == nil {
				return nil, errXClosed
			}
			if p.Type == x.a.incr {
				if p.Format != 32 || len(p.Value) != 4 {
					return nil, ErrUnsupported
				}
				if uint64(xgb.Get32(p.Value)) > uint64(limit) {
					return nil, ErrTooLarge
				}
				return x.receiveINCR(ctx, w, limit, revision)
			}
			if p.BytesAfter != 0 || len(p.Value) > limit {
				return nil, ErrTooLarge
			}
			if p.Type == 0 {
				return nil, ErrUnsupported
			}
			return p, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, context.DeadlineExceeded
		case <-tick.C:
		}
	}
}

// The INCR header is only a lower bound, never an allocation size. Every chunk
// is read with the remaining format budget, and BytesAfter detects truncation.
// A missing zero-length terminator, format changes and stalled sources fail.
func (x *xClipboard) receiveINCR(ctx context.Context, w xproto.Window, limit int, revision uint64) (*xproto.GetPropertyReply, error) {
	if err := xproto.DeletePropertyChecked(x.c, w, x.a.property).Check(); err != nil {
		return nil, err
	}
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	deadline := time.Now().Add(2 * time.Second)
	var result xproto.GetPropertyReply
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		x.drain(nil)
		if x.revision != revision {
			return nil, ErrChanged
		}
		remaining := limit - len(result.Value)
		units := uint32(max(1, (remaining+3)/4))
		p, err := xproto.GetProperty(x.c, true, w, x.a.property, xproto.GetPropertyTypeAny, 0, units).Reply()
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, errXClosed
		}
		if p.Type != 0 {
			if p.BytesAfter != 0 || len(p.Value) > remaining {
				return nil, ErrTooLarge
			}
			if p.Type == x.a.incr || (p.Format != 8 && p.Format != 32) {
				return nil, ErrUnsupported
			}
			if result.Type == 0 {
				result.Type, result.Format = p.Type, p.Format
			}
			if result.Type != p.Type || result.Format != p.Format {
				return nil, ErrUnsupported
			}
			if len(p.Value) == 0 {
				result.ValueLen = uint32(len(result.Value) / (int(result.Format) / 8))
				return &result, nil
			}
			result.Value = append(result.Value, p.Value...)
			deadline = time.Now().Add(2 * time.Second)
			continue
		}
		if !time.Now().Before(deadline) {
			return nil, context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}
