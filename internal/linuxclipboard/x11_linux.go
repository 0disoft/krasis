package linuxclipboard

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/x11registry"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
)

type atoms struct{ clipboard, targets, utf8, text, plain, png, incr, property, bridge xproto.Atom }
type nativeResult struct {
	observation
	value peerupdate.ClipboardValue
	err   error
}
type nativeCall struct {
	ctx    context.Context
	budget time.Duration
	run    func(context.Context) nativeResult
	result chan nativeResult
}
type xClipboard struct {
	c         *xgb.Conn
	wire      net.Conn
	window    xproto.Window
	a         atoms
	revision  uint64
	owner     xproto.Window
	value     peerupdate.ClipboardValue
	calls     chan nativeCall
	done      chan struct{}
	cancel    context.CancelFunc
	chunk     int
	transfers map[transferKey]*outgoing
}

func openClipboard(ctx context.Context, display string) (clipboard, error) {
	c, wire, err := connectX(ctx, display)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (clipboard, error) { _ = wire.Close(); c.Close(); return nil, err }
	stop := context.AfterFunc(ctx, func() { _ = wire.Close() })
	defer stop()
	if err = x11registry.InitXfixes(c); err != nil {
		return fail(err)
	}
	x := &xClipboard{c: c, wire: wire, revision: 1, calls: make(chan nativeCall), done: make(chan struct{}), transfers: make(map[transferKey]*outgoing)}
	for _, entry := range []struct {
		name string
		atom *xproto.Atom
	}{
		{"CLIPBOARD", &x.a.clipboard}, {"TARGETS", &x.a.targets}, {"UTF8_STRING", &x.a.utf8},
		{"TEXT", &x.a.text}, {"text/plain;charset=utf-8", &x.a.plain}, {"image/png", &x.a.png},
		{"INCR", &x.a.incr}, {"_KRASIS_CLIPBOARD", &x.a.property},
		{"_KRASIS_CLIPBOARD_BRIDGE", &x.a.bridge},
	} {
		r, atomErr := xproto.InternAtom(c, false, uint16(len(entry.name)), entry.name).Reply()
		if atomErr != nil {
			return fail(atomErr)
		}
		if r == nil {
			return fail(errXClosed)
		}
		*entry.atom = r.Atom
	}
	if x.window, err = x.newWindow(); err != nil {
		return fail(err)
	}
	if err = x.claimBridge(); err != nil {
		return fail(err)
	}
	mask := uint32(xfixes.SelectionEventMaskSetSelectionOwner | xfixes.SelectionEventMaskSelectionWindowDestroy | xfixes.SelectionEventMaskSelectionClientClose)
	if err = xfixes.SelectSelectionInputChecked(c, x.window, x.a.clipboard, mask).Check(); err != nil {
		return fail(err)
	}
	if _, err = x.current(); err != nil {
		return fail(err)
	}
	// No BIG-REQUESTS dependency: every chunk fits the core request maximum.
	x.chunk = min(64<<10, int(xproto.Setup(c).MaximumRequestLength)*4-28)
	if x.chunk < 1024 {
		return fail(errors.New("X11 maximum request size too small"))
	}
	loopCtx, cancel := context.WithCancel(ctx)
	x.cancel = cancel
	_ = wire.SetDeadline(time.Time{})
	go x.loop(loopCtx)
	return x, nil
}

// A display owns one bridge, including when multiple CLI invocations race to
// start it. The X server releases this lease automatically on disconnect.
func (x *xClipboard) claimBridge() error {
	xproto.GrabServer(x.c)
	defer func() { _ = xproto.UngrabServerChecked(x.c).Check() }()
	r, err := xproto.GetSelectionOwner(x.c, x.a.bridge).Reply()
	if err != nil {
		return err
	}
	if r == nil {
		return errXClosed
	}
	if r.Owner != 0 {
		return ErrAlreadyRunning
	}
	return xproto.SetSelectionOwnerChecked(x.c, x.window, x.a.bridge, xproto.TimeCurrentTime).Check()
}

func (x *xClipboard) close() { x.cancel(); _ = x.wire.Close(); <-x.done }

func (x *xClipboard) call(ctx context.Context, budget time.Duration, run func(context.Context) nativeResult) nativeResult {
	if err := ctx.Err(); err != nil {
		return nativeResult{err: err}
	}
	call := nativeCall{ctx: ctx, budget: budget, run: run, result: make(chan nativeResult, 1)}
	select {
	case <-ctx.Done():
		return nativeResult{err: ctx.Err()}
	case <-x.done:
		return nativeResult{err: errXClosed}
	case x.calls <- call:
	}
	select {
	case <-ctx.Done():
		return nativeResult{err: ctx.Err()}
	case <-x.done:
		return nativeResult{err: errXClosed}
	case r := <-call.result:
		return r
	}
}

func (x *xClipboard) observe(ctx context.Context) (observation, error) {
	r := x.call(ctx, xTimeout, func(ctx context.Context) nativeResult {
		o, err := x.current()
		return nativeResult{observation: o, err: err}
	})
	return r.observation, r.err
}
func (x *xClipboard) read(ctx context.Context, revision uint64) (peerupdate.ClipboardValue, error) {
	r := x.call(ctx, 10*time.Second, func(ctx context.Context) nativeResult {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		v, err := x.readSelection(ctx, revision)
		return nativeResult{value: v, err: err}
	})
	return r.value, r.err
}
func (x *xClipboard) write(ctx context.Context, revision uint64, v peerupdate.ClipboardValue) (observation, error) {
	r := x.call(ctx, xTimeout, func(ctx context.Context) nativeResult {
		o, err := x.own(ctx, revision, v)
		return nativeResult{observation: o, err: err}
	})
	return r.observation, r.err
}

func (x *xClipboard) loop(ctx context.Context) {
	defer close(x.done)
	defer x.c.Close()
	defer x.wire.Close()
	stop := context.AfterFunc(ctx, func() { _ = x.wire.Close() })
	defer stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case call := <-x.calls:
			if err := call.ctx.Err(); err != nil {
				call.result <- nativeResult{err: err}
				continue
			}
			deadline := time.Now().Add(call.budget)
			if until, ok := call.ctx.Deadline(); ok && until.Before(deadline) {
				deadline = until
			}
			_ = x.wire.SetDeadline(deadline)
			// A canceled blocking X request poisons this connection rather than
			// leaving a late native write running after the caller has returned.
			stopCall := context.AfterFunc(call.ctx, func() { _ = x.wire.Close() })
			r := call.run(call.ctx)
			stopCall()
			_ = x.wire.SetDeadline(time.Time{})
			call.result <- r
		case <-ticker.C:
			_ = x.wire.SetDeadline(time.Now().Add(xTimeout))
			x.drain(nil)
			x.expireTransfers(time.Now())
			_ = x.wire.SetDeadline(time.Time{})
		}
	}
}

func (x *xClipboard) newWindow() (xproto.Window, error) {
	w, err := xproto.NewWindowId(x.c)
	if err != nil {
		return 0, err
	}
	err = xproto.CreateWindowChecked(x.c, 0, w, xproto.Setup(x.c).DefaultScreen(x.c).Root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check()
	return w, err
}

func (x *xClipboard) current() (observation, error) {
	r, err := xproto.GetSelectionOwner(x.c, x.a.clipboard).Reply()
	if err != nil {
		return observation{}, err
	}
	if r == nil {
		return observation{}, errXClosed
	}
	if r.Owner != x.owner {
		x.owner = r.Owner
		x.revision++
	}
	x.drain(nil) // Events preceding this reply are queued; newer events win too.
	return observation{x.revision, x.owner == x.window}, nil
}

// drain bounds work per turn, including hostile clients flooding requests.
func (x *xClipboard) drain(notify func(xproto.SelectionNotifyEvent)) {
	for range 256 {
		event, _ := x.c.PollForEvent()
		if event == nil {
			return
		}
		switch e := event.(type) {
		case xfixes.SelectionNotifyEvent:
			if e.Selection == x.a.clipboard && e.Window == x.window {
				x.owner = e.Owner
				x.revision++
				if e.Owner != x.window {
					x.value = peerupdate.ClipboardValue{}
				}
			}
		case xproto.SelectionClearEvent:
			if e.Selection == x.a.clipboard && e.Owner == x.window {
				x.value = peerupdate.ClipboardValue{}
			}
		case xproto.SelectionRequestEvent:
			x.serve(e)
		case xproto.PropertyNotifyEvent:
			x.advance(e)
		case xproto.SelectionNotifyEvent:
			if notify != nil {
				notify(e)
			}
		case xproto.DestroyNotifyEvent:
			x.dropWindow(e.Window)
		}
	}
}

func (x *xClipboard) own(ctx context.Context, revision uint64, v peerupdate.ClipboardValue) (observation, error) {
	if err := validateValue(v); err != nil {
		return observation{}, err
	}
	if err := ctx.Err(); err != nil {
		return observation{}, err
	}
	// Briefly freeze other clients while checking the revision and acquiring
	// ownership; a copy during the HTTP exchange must win over an incoming paste.
	xproto.GrabServer(x.c)
	defer func() { _ = xproto.UngrabServerChecked(x.c).Check() }()
	current, err := x.current()
	if err != nil {
		return observation{}, err
	}
	if current.revision != revision {
		return observation{}, ErrChanged
	}
	if err := ctx.Err(); err != nil {
		return observation{}, err
	}
	if err := xproto.SetSelectionOwnerChecked(x.c, x.window, x.a.clipboard, xproto.TimeCurrentTime).Check(); err != nil {
		return observation{}, err
	}
	current, err = x.current()
	if err != nil {
		return observation{}, err
	}
	if !current.owned {
		return observation{}, ErrChanged
	}
	x.value = v // Immutable; outstanding INCR transfers retain their own slice.
	return current, nil
}
