package x11desktop

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/x11registry"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xinerama"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

type Geometry struct{ Count, Width, Height int32 }

// A fresh read at authority boundaries, without initializing input extensions.
func QueryGeometry(ctx context.Context) (Geometry, error) {
	c, err := openConnection(ctx)
	if err != nil {
		return Geometry{}, err
	}
	defer c.close()
	d := &Desktop{c: c}
	var g Geometry
	err = c.do(ctx, func() error { var e error; g, e = d.readGeometry(); return e })
	return g, err
}

type Desktop struct {
	c                *connection
	geometry         Geometry
	keys             map[byte]uint32
	codes            map[uint32]byte
	xiOpcode         byte
	pointerDevice    uint16
	closeOnce        sync.Once
	closeErr         error
	wheelX, wheelY   int64 // c.mu
	grabbed, hidden  bool  // c.mu
	beforeX, beforeY int16
}

func Open(ctx context.Context) (*Desktop, error) {
	c, err := openConnection(ctx)
	if err != nil {
		return nil, err
	}
	d := &Desktop{c: c}
	err = c.do(ctx, func() error {
		if err := xtest.Init(c.x); err != nil {
			return fmt.Errorf("XTEST required: %w", err)
		}
		if err := x11registry.InitXfixes(c.x); err != nil {
			return err
		}
		v, err := xfixes.QueryVersion(c.x, 4, 0).Reply()
		if err != nil {
			return err
		}
		if v.MajorVersion < 4 {
			return errors.New("XFIXES 4 required")
		}
		d.geometry, err = d.readGeometry()
		if err != nil {
			return err
		}
		if d.geometry.Count != 1 || d.geometry.Width < 3 || d.geometry.Height < 3 || d.geometry.Width > 32767 || d.geometry.Height > 32767 {
			return errors.New("one X11 monitor with dimensions 3..32767 is required")
		}
		d.keys, d.codes, err = readPhysicalKeys(c.x)
		if err != nil {
			return err
		}
		return d.initPointerState()
	})
	if err != nil {
		c.close()
		return nil, err
	}
	return d, nil
}

func (d *Desktop) Close() error {
	if d != nil {
		d.closeOnce.Do(func() { d.closeErr = d.c.close() })
		return d.closeErr
	}
	return nil
}

func (d *Desktop) readGeometry() (Geometry, error) {
	g, err := xproto.GetGeometry(d.c.x, xproto.Drawable(d.c.root)).Reply()
	if err != nil {
		return Geometry{}, err
	}
	count := int32(1)
	if err := xinerama.Init(d.c.x); err == nil {
		s, err := xinerama.QueryScreens(d.c.x).Reply()
		if err != nil {
			return Geometry{}, err
		}
		if len(s.ScreenInfo) > 0 {
			count = int32(len(s.ScreenInfo))
		}
	}
	return Geometry{count, int32(g.Width), int32(g.Height)}, nil
}

func (d *Desktop) Geometry(ctx context.Context) (g Geometry, err error) {
	err = d.c.do(ctx, func() error { var e error; g, e = d.readGeometry(); return e })
	return
}
func (d *Desktop) Position(ctx context.Context) (x, y int32, err error) {
	err = d.c.do(ctx, func() error {
		p, e := xproto.QueryPointer(d.c.x, d.c.root).Reply()
		if e == nil {
			x, y = int32(p.RootX), int32(p.RootY)
		}
		return e
	})
	return
}
func (d *Desktop) SetPosition(ctx context.Context, x, y int32) error {
	if x < 0 || y < 0 || x >= d.geometry.Width || y >= d.geometry.Height {
		return errors.New("cursor outside X11 monitor")
	}
	return d.c.do(ctx, func() error {
		return xproto.WarpPointerChecked(d.c.x, 0, d.c.root, 0, 0, 0, 0, int16(x), int16(y)).Check()
	})
}

func (d *Desktop) Key(ctx context.Context, code uint32, down bool) error {
	key, ok := d.codes[code]
	if !ok {
		return fmt.Errorf("unsupported physical scan code %x", code)
	}
	return d.c.do(ctx, func() error {
		kind := byte(xproto.KeyRelease)
		if down {
			kind = xproto.KeyPress
		}
		return xtest.FakeInputChecked(d.c.x, kind, key, 0, 0, 0, 0, 0).Check()
	})
}

func Button(code uint32) (byte, error) {
	switch code {
	case 1:
		return 1, nil
	case 2:
		return 3, nil
	case 3:
		return 2, nil
	case 4:
		return 8, nil
	case 5:
		return 9, nil
	}
	return 0, errors.New("unsupported mouse button")
}
func (d *Desktop) Button(ctx context.Context, code uint32, down bool) error {
	b, err := Button(code)
	if err != nil {
		return err
	}
	return d.c.do(ctx, func() error {
		kind := byte(xproto.ButtonRelease)
		if down {
			kind = xproto.ButtonPress
		}
		return xtest.FakeInputChecked(d.c.x, kind, b, 0, 0, 0, 0, 0).Check()
	})
}

func (d *Desktop) Move(ctx context.Context, dx, dy int32) (x, y int32, err error) {
	if dx < -control.MaxInputDelta || dx > control.MaxInputDelta || dy < -control.MaxInputDelta || dy > control.MaxInputDelta || dx == 0 && dy == 0 {
		return 0, 0, errors.New("invalid relative motion")
	}
	err = d.c.do(ctx, func() error {
		// Motion detail=1 selects relative coordinates; time is a delay, not
		// the coordinate mode. Never add a per-event delay on the input lane.
		if e := xtest.FakeInputChecked(d.c.x, xproto.MotionNotify, 1, 0, 0, int16(dx), int16(dy), 0).Check(); e != nil {
			return e
		}
		p, e := xproto.QueryPointer(d.c.x, d.c.root).Reply()
		if e == nil {
			x, y = int32(p.RootX), int32(p.RootY)
		}
		return e
	})
	return
}

func (d *Desktop) Wheel(ctx context.Context, dx, dy int32) error {
	if dx < -control.MaxInputDelta || dx > control.MaxInputDelta || dy < -control.MaxInputDelta || dy > control.MaxInputDelta || dx == 0 && dy == 0 {
		return errors.New("invalid wheel delta")
	}
	return d.c.do(ctx, func() error {
		d.wheelX += int64(dx)
		d.wheelY += int64(dy)
		for _, axis := range []struct {
			value              *int64
			positive, negative byte
		}{{&d.wheelY, 4, 5}, {&d.wheelX, 7, 6}} {
			for *axis.value >= 120 || *axis.value <= -120 {
				b := axis.positive
				step := int64(120)
				if *axis.value < 0 {
					b = axis.negative
					step = -120
				}
				if err := xtest.FakeInputChecked(d.c.x, xproto.ButtonPress, b, 0, 0, 0, 0, 0).Check(); err != nil {
					return err
				}
				if err := xtest.FakeInputChecked(d.c.x, xproto.ButtonRelease, b, 0, 0, 0, 0, 0).Check(); err != nil {
					return err
				}
				*axis.value -= step
			}
		}
		return nil
	})
}

func (d *Desktop) AnyPressed(ctx context.Context) (held bool, err error) {
	err = d.c.do(ctx, func() error {
		keys, e := xproto.QueryKeymap(d.c.x).Reply()
		if e != nil {
			return e
		}
		for _, b := range keys.Keys {
			if b != 0 {
				held = true
				return nil
			}
		}
		held, e = d.pointerPressed()
		return e
	})
	return
}

func (d *Desktop) Grab(ctx context.Context) error {
	return d.c.do(ctx, func() (err error) {
		if d.grabbed {
			return errors.New("X11 capture already active")
		}
		p, err := xproto.QueryPointer(d.c.x, d.c.root).Reply()
		if err != nil {
			return err
		}
		d.beforeX, d.beforeY = p.RootX, p.RootY
		k, err := xproto.GrabKeyboard(d.c.x, false, d.c.root, 0, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
		if err != nil {
			return err
		}
		if k.Status != xproto.GrabStatusSuccess {
			return errors.New("X11 keyboard is already grabbed")
		}
		defer func() {
			if err != nil {
				_ = xproto.UngrabKeyboardChecked(d.c.x, 0).Check()
				_ = xproto.UngrabPointerChecked(d.c.x, 0).Check()
			}
		}()
		g, err := xproto.GrabPointer(d.c.x, false, d.c.root, xproto.EventMaskPointerMotion|xproto.EventMaskButtonPress|xproto.EventMaskButtonRelease, xproto.GrabModeAsync, xproto.GrabModeAsync, d.c.root, 0, 0).Reply()
		if err != nil {
			return err
		}
		if g.Status != xproto.GrabStatusSuccess {
			return errors.New("X11 pointer is already grabbed")
		}
		// XFIXES counts hide requests per client. A return presentation may
		// already own one; do not leave an extra hide after the capture retires.
		if !d.hidden {
			if err = xfixes.HideCursorChecked(d.c.x, d.c.root).Check(); err != nil {
				return err
			}
			d.hidden = true
		}
		if err = xproto.WarpPointerChecked(d.c.x, 0, d.c.root, 0, 0, 0, 0, int16(d.geometry.Width/2), int16(d.geometry.Height/2)).Check(); err != nil {
			return err
		}
		d.grabbed = true
		return nil
	})
}

func (d *Desktop) Ungrab(ctx context.Context) error {
	return d.c.do(ctx, func() error {
		if !d.grabbed && !d.hidden {
			return nil
		}
		var errs []error
		if d.hidden {
			errs = append(errs, xfixes.ShowCursorChecked(d.c.x, d.c.root).Check())
			d.hidden = false
		}
		errs = append(errs, xproto.UngrabPointerChecked(d.c.x, 0).Check(), xproto.UngrabKeyboardChecked(d.c.x, 0).Check())
		if d.grabbed {
			errs = append(errs, xproto.WarpPointerChecked(d.c.x, 0, d.c.root, 0, 0, 0, 0, d.beforeX, d.beforeY).Check())
			d.grabbed = false
		}
		return errors.Join(errs...)
	})
}

// Poll never waits for an event. The caller owns event order, including warp
// events, and must drain a bounded batch so cancellation cannot be starved.
func (d *Desktop) Poll() (xgb.Event, error)     { return d.c.x.PollForEvent() }
func (d *Desktop) Scan(key byte) (uint32, bool) { code, ok := d.keys[key]; return code, ok }
func (d *Desktop) Center() (int32, int32)       { return d.geometry.Width / 2, d.geometry.Height / 2 }

func (d *Desktop) Dimensions() Geometry { return d.geometry }

func (d *Desktop) ShowCursor(ctx context.Context, visible bool) error {
	return d.c.do(ctx, func() error {
		if visible && d.hidden {
			if err := xfixes.ShowCursorChecked(d.c.x, d.c.root).Check(); err != nil {
				return err
			}
			d.hidden = false
		}
		if !visible && !d.hidden {
			if err := xfixes.HideCursorChecked(d.c.x, d.c.root).Check(); err != nil {
				return err
			}
			d.hidden = true
		}
		return nil
	})
}

func readPhysicalKeys(c *xgb.Conn) (map[byte]uint32, map[uint32]byte, error) {
	ext, err := xproto.QueryExtension(c, 9, "XKEYBOARD").Reply()
	if err != nil {
		return nil, nil, err
	}
	if !ext.Present {
		return nil, nil, errors.New("XKB extension required")
	}
	request := func(buf []byte) ([]byte, error) {
		cookie := c.NewCookie(true, true)
		c.NewRequest(buf, cookie)
		return cookie.Reply()
	}
	use := make([]byte, 8)
	use[0] = ext.MajorOpcode
	xgb.Put16(use[2:], 2)
	xgb.Put16(use[4:], 1)
	reply, err := request(use)
	if err != nil {
		return nil, nil, err
	}
	if len(reply) < 32 || reply[1] == 0 {
		return nil, nil, errors.New("XKB 1.0 unavailable")
	}
	get := make([]byte, 12)
	get[0], get[1] = ext.MajorOpcode, 17
	xgb.Put16(get[2:], 3)
	xgb.Put16(get[4:], 0x100)
	xgb.Put32(get[8:], 1<<9)
	reply, err = request(get)
	if err != nil {
		return nil, nil, err
	}
	if len(reply) < 32 || xgb.Get32(reply[8:]) != 1<<9 {
		return nil, nil, errors.New("XKB physical key names unavailable")
	}
	n := int(reply[19])
	if len(reply) < 32+n*4 {
		return nil, nil, errors.New("truncated XKB key names")
	}
	return keyMaps(int(reply[18]), reply[32:32+n*4])
}
