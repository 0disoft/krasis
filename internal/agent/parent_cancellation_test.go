package agent

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

type opaqueShutdownFailure struct{}

func (opaqueShutdownFailure) Error() string { return "opaque cleanup failure" }
func (opaqueShutdownFailure) Unwrap() error { return nil }

func TestRecoveredParentCancellationRequiresCompleteOwnerProof(t *testing.T) {
	parent := &agentlifecycle.TransitionError{Event: agentlifecycle.EventSessionLock}
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(parent)
	operation := errors.Join(peer.ErrPeerInputDispatch, io.EOF, context.Canceled)
	cause := errors.Join(parent, operation)
	clean := Result{FinalState: control.StateLocal, Recovery: RecoveryReport{
		ForwardingStopped: RecoveryConfirmed, WorkersJoined: RecoveryConfirmed,
		NativeOwnersReleased: RecoveryConfirmed, PressedLedgerEmpty: RecoveryConfirmed,
	}}
	for _, tc := range []struct {
		name   string
		mutate func(*Result)
	}{
		{"clean", func(*Result) {}},
		{"forwarding", func(r *Result) { r.Recovery.ForwardingStopped = RecoveryUnknown }},
		{"workers", func(r *Result) { r.Recovery.WorkersJoined = RecoveryUnknown }},
		{"native", func(r *Result) { r.Recovery.NativeOwnersReleased = RecoveryUnknown }},
		{"pressed", func(r *Result) { r.Recovery.PressedLedgerEmpty = RecoveryUnknown }},
		{"expired", func(r *Result) { r.Recovery.DeadlineExceeded = true }},
		{"nonlocal", func(r *Result) { r.FinalState = control.StateFault }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := clean
			tc.mutate(&r)
			err := recoveredParentCancellation(ctx, parent, r, cause)
			if RecoveredParentCancellation(err, parent) != (tc.name == "clean") || !errors.Is(err, io.EOF) || !errors.Is(err, peer.ErrPeerInputDispatch) {
				t.Fatal("lost failure or accepted incomplete proof", err)
			}
			if RecoveredParentCancellation(errors.Join(err, errors.New("guard close failed")), parent) {
				t.Fatal("accepted joined cleanup error")
			}
		})
	}
	for _, first := range []error{io.EOF, context.Canceled} {
		if err := recoveredParentCancellation(ctx, first, clean, cause); RecoveredParentCancellation(err, parent) {
			t.Fatal("component-first fault misclassified as parent stop", err)
		}
	}
	if err := recoveredParentCancellation(ctx, parent, clean, errors.Join(cause, ErrRecoveryUnverified)); RecoveredParentCancellation(err, parent) {
		t.Fatal("accepted unverified cleanup", err)
	}
	for _, extra := range []error{transport.ErrInvalidFrame, peer.ErrPeerLivenessExpired, errors.New("guard close failed"), opaqueShutdownFailure{},
		&application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: context.Canceled},
		application.TokenReleaseError{Err: context.Canceled}, application.EffectFailure{Err: context.Canceled},
		&transport.FrameWriteFailure{Cause: context.Canceled}, &peer.ControlDispatchFailure{Cause: context.Canceled},
	} {
		err := recoveredParentCancellation(ctx, parent, clean, errors.Join(cause, extra))
		if RecoveredParentCancellation(err, parent) || !errors.Is(err, extra) {
			t.Fatal("masked unrelated late failure", err)
		}
	}
	if RecoveredParentCancellation(recoveredParentCancellation(ctx, parent, clean, cause), errors.New(parent.Error())) {
		t.Fatal("accepted different parent cause with same message")
	}
	for _, other := range []error{context.Canceled, errors.New("manual disconnect"), &agentlifecycle.TransitionError{Event: "invalid"}} {
		stopped, stop := context.WithCancelCause(context.Background())
		stop(other)
		original := errors.Join(other, context.Canceled)
		if got := recoveredParentCancellation(stopped, other, clean, original); got != original {
			t.Fatal("changed non-lifecycle cancellation contract", got)
		}
	}
}
