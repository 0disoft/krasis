package loginagent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/control"
)

func TestLoginSessionRetirementRequiresEveryRecoveryProof(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	confirmed := agent.Result{FinalState: control.StateLocal, Recovery: agent.RecoveryReport{
		ForwardingStopped: agent.RecoveryConfirmed, WorkersJoined: agent.RecoveryConfirmed,
		NativeOwnersReleased: agent.RecoveryConfirmed, PressedLedgerEmpty: agent.RecoveryConfirmed,
	}}
	cause := errors.New("connection interrupted while stopping")
	if err := loginSessionRetirement(ctx, confirmed, cause); err != context.Canceled {
		t.Fatal(err)
	}
	for _, damage := range []func(*agent.Result){
		func(r *agent.Result) { r.Recovery.ForwardingStopped = agent.RecoveryUnknown },
		func(r *agent.Result) { r.Recovery.WorkersJoined = agent.RecoveryUnknown },
		func(r *agent.Result) { r.Recovery.NativeOwnersReleased = agent.RecoveryUnknown },
		func(r *agent.Result) { r.Recovery.PressedLedgerEmpty = agent.RecoveryUnknown },
		func(r *agent.Result) { r.Recovery.DeadlineExceeded = true },
		func(r *agent.Result) { r.FinalState = control.StateFault },
	} {
		broken := confirmed
		damage(&broken)
		if err := loginSessionRetirement(ctx, broken, cause); err != cause {
			t.Fatal("incomplete cleanup became clean", err)
		}
	}
	if err := loginSessionRetirement(context.Background(), confirmed, cause); err != cause {
		t.Fatal("live failure was erased")
	}
	uncertain := errors.Join(context.Canceled, agent.ErrRecoveryUnverified)
	if err := loginSessionRetirement(ctx, confirmed, uncertain); err != uncertain {
		t.Fatal("unverified recovery was erased")
	}
}

func TestLoginStopClassificationDoesNotDiscardJoinedFailures(t *testing.T) {
	for _, err := range []error{context.Canceled, fmt.Errorf("monitor: %w", ErrGrant), errors.Join(context.Canceled, ErrObservation), errors.Join(context.DeadlineExceeded, ErrSuspended)} {
		if !loginStopOnly(err) {
			t.Fatal("expected policy stop refused", err)
		}
	}
	for _, err := range []error{agent.ErrRecoveryUnverified, errors.Join(context.Canceled, errors.New("native close failed")), errors.Join(ErrGrant, agent.ErrRecoveryUnverified)} {
		if loginStopOnly(err) {
			t.Fatal("cleanup failure became clean", err)
		}
	}
}
