//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type profiledSourceCaptureStub struct {
	err error
}

func (s *profiledSourceCaptureStub) Activate() error { panic("owner profile context was dropped") }
func (s *profiledSourceCaptureStub) Deactivate()     {}
func (s *profiledSourceCaptureStub) Active() bool    { return s.err == nil }
func (s *profiledSourceCaptureStub) ActivateProfile(ctx context.Context) error {
	handoff.RecordCapture(ctx, handoff.CaptureQueueToHandle, 7*time.Millisecond)
	handoff.RecordCapture(ctx, handoff.CaptureForeground, 321*time.Millisecond)
	if s.err != nil {
		handoff.RecordCapture(ctx, handoff.CaptureRollback, 2*time.Millisecond)
	}
	return s.err
}

func TestProtectedSourceActivationCarriesOwnerStagesWithoutChangingOutcome(t *testing.T) {
	for _, failure := range []error{nil, errors.New("synthetic native activation rejection")} {
		detail, err := activateSourceCaptureProfile(&profiledSourceCaptureStub{err: failure})
		if err != failure || !detail.Valid() || detail[handoff.CaptureQueueToHandle] != 2 || detail[handoff.CaptureForeground] != 4 || (detail[handoff.CaptureRollback] != 0) != (failure != nil) {
			t.Fatal("protected owner lost stage timing or changed activation outcome", detail, err)
		}
	}
}
