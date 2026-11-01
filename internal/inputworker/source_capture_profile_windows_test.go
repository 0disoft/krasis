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
	handoff.RecordCaptureExact(ctx, handoff.CaptureExactQueueToHandle, 7*time.Millisecond)
	handoff.RecordCaptureExact(ctx, handoff.CaptureExactForegroundRequest, 210*time.Millisecond)
	handoff.RecordCaptureExact(ctx, handoff.CaptureExactForegroundVerify, 111*time.Millisecond)
	if s.err != nil {
		handoff.RecordCapture(ctx, handoff.CaptureRollback, 2*time.Millisecond)
	}
	return s.err
}

func TestProtectedSourceActivationCarriesOwnerStagesWithoutChangingOutcome(t *testing.T) {
	for _, failure := range []error{nil, errors.New("synthetic native activation rejection")} {
		detail, exact, err := activateSourceCaptureProfile(&profiledSourceCaptureStub{err: failure})
		if err != failure || !detail.Valid() || detail[handoff.CaptureQueueToHandle] != 2 || detail[handoff.CaptureForeground] != 4 || (detail[handoff.CaptureRollback] != 0) != (failure != nil) {
			t.Fatal("protected owner lost stage timing or changed activation outcome", detail, err)
		}
		if !exact.Valid() || exact.Measured != 1<<handoff.CaptureExactQueueToHandle|1<<handoff.CaptureExactForegroundRequest|1<<handoff.CaptureExactForegroundVerify || exact.QueueToHandleMicros != 7000 || exact.ForegroundRequestMicros != 210000 || exact.ForegroundVerifyMicros != 111000 {
			t.Fatal("protected owner lost exact stage timing or changed outcome", exact, err)
		}
	}
}
