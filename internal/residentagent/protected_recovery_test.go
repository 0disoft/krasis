package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestProtectedInputRecoversOnlyAfterPromptCloses(t *testing.T) {
	finished := errors.New("second session completed")
	attempts, waits, probes := 0, 0, 0
	operations := testOperations(t, func(_ context.Context, _ string, observer agent.SupervisorObserver) error {
		attempts++
		if attempts == 1 {
			return errors.Join(context.Canceled, wininput.ErrProtectedInput)
		}
		observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorAcquiring})
		return finished
	})
	done := make(chan struct{})
	operations.sessionDone = done
	operations.sessionOutcome = &sessionOutcome{connected: true}
	operations.sessionState = "CONNECTED"
	observer := &sessionObserver{operations: operations, done: done}
	err := operations.runWithProtectedRecovery(context.Background(), "peer", observer, func() bool {
		probes++
		return probes == 1
	}, func(_ context.Context, delay time.Duration) error {
		waits++
		if delay != protectedRecoveryDelay || attempts != 1 {
			t.Fatalf("recovery waited=%s attempts=%d", delay, attempts)
		}
		status, _ := operations.Status(context.Background())
		if status.State != "RECONNECTING" {
			t.Fatalf("state during protected UI = %s", status.State)
		}
		return nil
	})
	if !errors.Is(err, finished) || attempts != 2 || waits != 2 || probes != 2 || !observer.retrying {
		t.Fatalf("result=%v attempts=%d waits=%d probes=%d retrying=%t", err, attempts, waits, probes, observer.retrying)
	}
	status, _ := operations.Status(context.Background())
	if status.State != "RECONNECTING" {
		t.Fatalf("new supervisor reset retry state: %s", status.State)
	}
}

func TestProtectedInputNeverRetriesAfterManualStopOrFailedCleanup(t *testing.T) {
	for _, test := range []struct {
		name       string
		manualStop bool
		failure    error
	}{
		{name: "manual stop", manualStop: true},
		{name: "failed cleanup", failure: errors.New("release failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			cause := errors.Join(context.Canceled, wininput.ErrProtectedInput, test.failure)
			operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
				attempts++
				return cause
			})
			operations.sessionDone = make(chan struct{})
			operations.manualStop = test.manualStop
			err := operations.runWithProtectedRecovery(context.Background(), "peer", &sessionObserver{operations: operations, done: operations.sessionDone}, func() bool {
				t.Fatal("unexpected protected probe")
				return false
			}, func(context.Context, time.Duration) error {
				t.Fatal("unexpected retry wait")
				return nil
			})
			if !errors.Is(err, cause) || attempts != 1 {
				t.Fatalf("result=%v attempts=%d", err, attempts)
			}
		})
	}
}
