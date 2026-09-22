package wininput

import (
	"errors"
	"testing"
)

type geometryCapture struct {
	active     bool
	onActivate func()
}

func (c *geometryCapture) Activate() error {
	c.active = true
	if c.onActivate != nil {
		c.onActivate()
	}
	return nil
}
func (c *geometryCapture) Deactivate()  { c.active = false }
func (c *geometryCapture) Active() bool { return c.active }

func TestGeometryGuardRollsBackCaptureChangedDuringActivation(t *testing.T) {
	changed := false
	native := &geometryCapture{onActivate: func() { changed = true }}
	capture := &guardedSourceActivation{SourceActivation: native, check: func() error {
		if changed {
			return ErrDisplayGeometryChanged
		}
		return nil
	}}
	if err := capture.Activate(); !errors.Is(err, ErrDisplayGeometryChanged) || native.active {
		t.Fatalf("stale capture active=%v err=%v", native.active, err)
	}
	if err := capture.Activate(); !errors.Is(err, ErrDisplayGeometryChanged) || native.active {
		t.Fatal("stale capture reactivated")
	}
}
