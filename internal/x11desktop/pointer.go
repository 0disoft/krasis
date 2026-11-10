package x11desktop

import (
	"errors"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Core QueryPointer exposes only buttons 1..5. XI2's complete button mask is
// required before capture so an already-held side button cannot acquire a
// remote release. No XI events or process-global decoders are registered.
func (d *Desktop) xiRequest(minor byte, payload []byte) ([]byte, error) {
	buf := make([]byte, 4+len(payload))
	buf[0], buf[1] = d.xiOpcode, minor
	xgb.Put16(buf[2:], uint16(len(buf)/4))
	copy(buf[4:], payload)
	cookie := d.c.x.NewCookie(true, true)
	d.c.x.NewRequest(buf, cookie)
	return cookie.Reply()
}
func (d *Desktop) initPointerState() error {
	ext, err := xproto.QueryExtension(d.c.x, 15, "XInputExtension").Reply()
	if err != nil {
		return err
	}
	if !ext.Present {
		return errors.New("XInput2 required for held-button admission")
	}
	d.xiOpcode = ext.MajorOpcode
	v := make([]byte, 4)
	xgb.Put16(v, 2)
	reply, err := d.xiRequest(47, v)
	if err != nil {
		return err
	}
	if len(reply) < 32 || xgb.Get16(reply[8:]) < 2 {
		return errors.New("XInput2 version unavailable")
	}
	p := make([]byte, 4) // Window=None: the pointer associated with this client.
	reply, err = d.xiRequest(45, p)
	if err != nil {
		return err
	}
	if len(reply) < 32 {
		return errors.New("truncated XInput client pointer")
	}
	d.pointerDevice = xgb.Get16(reply[10:])
	if d.pointerDevice < 2 {
		return errors.New("XInput client pointer unavailable")
	}
	_, err = d.pointerPressed()
	return err
}
func (d *Desktop) pointerPressed() (bool, error) {
	p := make([]byte, 8)
	xgb.Put32(p, uint32(d.c.root))
	xgb.Put16(p[4:], d.pointerDevice)
	reply, err := d.xiRequest(40, p)
	if err != nil {
		return false, err
	}
	return pointerMaskPressed(reply)
}
func pointerMaskPressed(reply []byte) (bool, error) {
	// XIQueryPointer: 56 bytes before its CARD32 button-mask list.
	if len(reply) < 56 {
		return false, errors.New("truncated XInput pointer state")
	}
	n := int(xgb.Get16(reply[34:]))
	if n > 256 || len(reply) < 56+4*n {
		return false, errors.New("invalid XInput button-mask bounds")
	}
	for _, b := range reply[56 : 56+4*n] {
		if b != 0 {
			return true, nil
		}
	}
	return false, nil
}
