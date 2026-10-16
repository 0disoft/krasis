package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestTrustFreshnessRecoversThroughNewRunnerAfterReaderReturns(t *testing.T) {
	finished := errors.New("fresh runner completed")
	attempts, waits := 0, 0
	operations := testOperations(t, func(_ context.Context, _ string, observer agent.SupervisorObserver) error {
		attempts++
		if attempts == 1 {
			trustReadSlot <- struct{}{}
			return errors.Join(context.Canceled, agent.ErrSessionTrustFreshness)
		}
		if len(trustReadSlot) != 0 {
			t.Fatal("new runner overlapped the old filesystem read")
		}
		observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorActive})
		return finished
	})
	operations.sessionDone = make(chan struct{})
	observer := &sessionObserver{operations: operations, done: operations.sessionDone}
	err := operations.runWithProtectedRecovery(context.Background(), "peer", observer, func() bool {
		t.Fatal("trust expiry queried the protected desktop")
		return false
	}, func(context.Context, time.Duration) error {
		waits++
		if attempts != 1 {
			t.Fatal("retry began before its bounded wait")
		}
		if waits == 2 {
			<-trustReadSlot
		}
		status, _ := operations.Status(context.Background())
		if status.State != "RECONNECTING" {
			t.Fatal("freshness exhausted UI recovery instead of keeping connection intent", status)
		}
		return nil
	})
	if len(trustReadSlot) != 0 {
		<-trustReadSlot
	}
	if !errors.Is(err, finished) || attempts != 2 || waits != 2 {
		t.Fatal(err, attempts, waits)
	}
}

func TestTrustFreshnessCannotRetryNegativeVerdictsOrUnverifiedCleanup(t *testing.T) {
	for _, failure := range []error{agent.ErrSessionTrustRevoked, agent.ErrSessionTrustMissing, truststore.ErrPeerRevoked, agent.ErrRecoveryUnverified, errors.New("private cleanup failure")} {
		attempts := 0
		cause := errors.Join(context.Canceled, agent.ErrSessionTrustFreshness, failure)
		operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
			attempts++
			return cause
		})
		operations.sessionDone = make(chan struct{})
		err := operations.runWithProtectedRecovery(context.Background(), "peer", &sessionObserver{operations: operations, done: operations.sessionDone}, func() bool { return false }, func(context.Context, time.Duration) error {
			t.Fatal("negative trust or failed cleanup retried")
			return nil
		})
		if !errors.Is(err, cause) || attempts != 1 {
			t.Fatal(err, attempts)
		}
	}
}
