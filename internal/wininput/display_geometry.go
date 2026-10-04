package wininput

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/handoff"
)

type DisplayGeometry struct{ Count, Left, Top, Width, Height int32 }

var ErrDisplayGeometryChanged = errors.New("display geometry changed during input session")

func (g DisplayGeometry) Valid() bool {
	return g.Count == 1 && g.Width > 2*cursorEntryInset && g.Height > 2*cursorEntryInset
}

// SetBoundaryGuard configures the immutable session geometry check before Run.
// It runs on admission/activation workers, never on low-level input callbacks.
func (h *SourceHook) SetBoundaryGuard(check func() error) { h.boundaryGuard = check }

type guardedSourceActivation struct {
	SourceActivation
	check func() error
}

func (s *guardedSourceActivation) Activate() error {
	return s.ActivateProfile(nil)
}

func (s *guardedSourceActivation) ActivateProfile(ctx context.Context) error {
	stopBefore := handoff.CaptureSpan(ctx, handoff.CaptureBoundaryBefore)
	err := s.check()
	stopBefore()
	if err != nil {
		return err
	}
	if capture, ok := s.SourceActivation.(SourceProfiledActivation); ok {
		err = capture.ActivateProfile(ctx)
	} else {
		err = s.SourceActivation.Activate()
	}
	if err != nil {
		return err
	}
	stopAfter := handoff.CaptureSpan(ctx, handoff.CaptureBoundaryAfter)
	err = s.check()
	stopAfter()
	if err != nil {
		s.SourceActivation.Deactivate()
		return err
	}
	return nil
}
