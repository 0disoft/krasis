package agent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/wininput"
	"testing"
	"time"
)

func TestUnverifiedRecoveryPreservesTerminalCaptureDescriptor(t *testing.T) {
	observer := &recordingSupervisorObserver{}
	failure := errors.Join(ErrRecoveryUnverified, wininput.ErrSourceCaptureThreadAttachFailed)
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now,
		Wait: func(context.Context, time.Duration) error { t.Fatal("retried unverified recovery"); return nil }, Observer: observer,
	}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, failure
		},
	})
	if !errors.Is(err, ErrRecoveryUnverified) {
		t.Fatal(err)
	}
	last := observer.snapshots[len(observer.snapshots)-1]
	if last.State != SupervisorStopped || last.FailureKind != "session" || last.FailureComponent != "source-capture" || last.FailureCode != "thread_attach_failed" {
		t.Fatalf("terminal reason lost: %+v", last)
	}
}

func TestSupervisorExitReasonPreservesUnderlyingCause(t *testing.T) {
	for _, phase := range []string{"source-close", "connection-close", "nonlocal-state", "unexpected-stop", "nil-connection"} {
		err := supervisorExit(phase, context.Canceled)
		if !errors.Is(err, context.Canceled) || errors.Unwrap(err) != context.Canceled || SupervisorExitPhase(err) != phase {
			t.Fatalf("phase=%s error=%v", phase, err)
		}
	}
	if SupervisorExitPhase(supervisorExit("private window title", errors.New("private path"))) != "" {
		t.Fatal("private phase escaped")
	}
}

func TestSupervisorNonLocalExitKeepsFailureAndStopReason(t *testing.T) {
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now,
		Wait: func(context.Context, time.Duration) error { t.Fatal("retried nonlocal state"); return nil },
	}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{}, wininput.ErrSourceCaptureLostForeground
		},
	})
	if SupervisorExitPhase(err) != "nonlocal-state" || !errors.Is(err, wininput.ErrSourceCaptureLostForeground) {
		t.Fatalf("cause lost: %v", err)
	}
}
