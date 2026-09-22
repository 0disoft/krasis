package wininput

import "errors"

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
	if err := s.check(); err != nil {
		return err
	}
	if err := s.SourceActivation.Activate(); err != nil {
		return err
	}
	if err := s.check(); err != nil {
		s.SourceActivation.Deactivate()
		return err
	}
	return nil
}
