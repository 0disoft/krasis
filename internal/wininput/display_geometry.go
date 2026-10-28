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
	check            func() error
	beforeAuthorized func() bool // Live command scope only; never a retained proof.
}

func (s *guardedSourceActivation) Activate() error {
	return s.ActivateProfile(nil)
}

func (s *guardedSourceActivation) ActivateProfile(ctx context.Context) error {
	var err error
	if s.beforeAuthorized == nil || !s.beforeAuthorized() {
		stopBefore := handoff.CaptureSpan(ctx, handoff.CaptureBoundaryBefore)
		err = s.check()
		stopBefore()
		if err != nil {
			return err
		}
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
		s.DeactivateProfile(ctx)
		return err
	}
	return nil
}

func (s *guardedSourceActivation) DeactivateProfile(ctx context.Context) {
	if capture, ok := s.SourceActivation.(SourceProfiledDeactivation); ok {
		capture.DeactivateProfile(ctx)
	} else {
		s.SourceActivation.Deactivate()
	}
}
