package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
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

type profiledGeometryCapture struct{ geometryCapture }

func (capture *profiledGeometryCapture) ActivateProfile(ctx context.Context) error {
	handoff.RecordCapture(ctx, handoff.CaptureShow, time.Millisecond)
	return capture.geometryCapture.Activate()
}

func (capture *profiledGeometryCapture) DeactivateProfile(ctx context.Context) {
	handoff.RecordFocusResult(ctx, 0, 1)
	capture.geometryCapture.Deactivate()
}

func TestProfiledGeometryGuardForwardsContextAndStillRollsBack(t *testing.T) {
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	checks := 0
	native := &profiledGeometryCapture{}
	capture := &guardedSourceActivation{SourceActivation: native, check: func() error {
		checks++
		if native.active {
			return ErrDisplayGeometryChanged
		}
		return nil
	}}
	if err := capture.ActivateProfile(p); !errors.Is(err, ErrDisplayGeometryChanged) || native.active || checks != 2 {
		t.Fatal("profiled guard skipped validation or recovery")
	}
	handoff.Finish(p, "error")
	mask := uint32(1<<handoff.CaptureShow | 1<<handoff.CaptureBoundaryBefore | 1<<handoff.CaptureBoundaryAfter)
	if !summary.Valid() || summary.Capture == nil || summary.Capture.Measured != mask || summary.Capture.Micros[handoff.CaptureShow] != 1000 {
		t.Fatal("guard swallowed profile context or partial evidence", summary)
	}
	if summary.Focus == nil || summary.Focus.Restore != 1 {
		t.Fatal("geometry rollback lost activation attempt context")
	}
}
