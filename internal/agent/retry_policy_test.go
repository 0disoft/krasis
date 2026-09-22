package agent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"testing"
	"time"
)

func TestRetryHasNoThreeAttemptLimitAndPublishesActualPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observer := &recordingSupervisorObserver{}
	source := &scriptedSource{outcomes: []sourceOutcome{{err: errors.New("offline")}, {err: errors.New("offline")}, {err: errors.New("offline")}, {connection: &stubConnection{}}}}
	runs := 0
	policy := DefaultBackoffPolicy()
	policy.Jitter = true
	err := RunSupervisor(ctx, SupervisorConfig{Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: policy, Now: time.Now, Observer: observer, Wait: func(context.Context, time.Duration) error { return nil }}, SupervisorDependencies{Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}, RunSession: func(context.Context, Config, Dependencies) (Result, error) {
		runs++
		cancel()
		return Result{FinalState: control.StateLocal}, context.Canceled
	}})
	if !errors.Is(err, context.Canceled) || runs != 1 {
		t.Fatalf("three failures blocked recovery: runs=%d error=%v", runs, err)
	}
	count := 0
	for _, snapshot := range observer.snapshotsCopy() {
		if snapshot.State == SupervisorBackoff {
			count++
			if snapshot.BackoffPolicy != policy || snapshot.Attempt != uint32(count) || snapshot.RetryAfter <= 0 {
				t.Fatalf("policy drift: %+v", snapshot)
			}
		}
	}
	if count != 3 {
		t.Fatalf("backoffs=%d", count)
	}
}

func TestJitterIsBoundedAndNeverExceedsMaximum(t *testing.T) {
	policy := DefaultBackoffPolicy()
	policy.Jitter = true
	for attempt := uint32(1); attempt <= 20; attempt++ {
		deterministic := policy
		deterministic.Jitter = false
		base, _ := deterministic.Delay(attempt)
		for i := 0; i < 20; i++ {
			delay, err := policy.Delay(attempt)
			if err != nil || delay < base*8/10 || delay > base*12/10 || delay > policy.Maximum {
				t.Fatalf("invalid jitter %v / %v", delay, err)
			}
		}
	}
}

func TestStableConnectionResetsAttemptWithExplicitEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Unix(100, 0)
	observer := &recordingSupervisorObserver{}
	waits := 0
	source := &scriptedSource{outcomes: []sourceOutcome{{err: errors.New("offline")}, {connection: &stubConnection{}}}}
	policy := DefaultBackoffPolicy()
	_ = RunSupervisor(ctx, SupervisorConfig{Session: Config{QueueCapacity: 1, Now: func() time.Time { return now }}, Backoff: policy, Now: func() time.Time { return now }, Observer: observer, Wait: func(context.Context, time.Duration) error {
		waits++
		if waits == 2 {
			cancel()
			return context.Canceled
		}
		return nil
	}}, SupervisorDependencies{Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}, RunSession: func(context.Context, Config, Dependencies) (Result, error) {
		now = now.Add(policy.StableAfter)
		return Result{FinalState: control.StateLocal}, errors.New("lost")
	}})
	found := false
	for _, s := range observer.snapshotsCopy() {
		if s.State == SupervisorBackoff && s.FailureKind == "session" {
			found = true
			if !s.StableReset || s.Attempt != 1 || s.RetryAfter != policy.Initial {
				t.Fatalf("reset evidence missing: %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("missing reset snapshot")
	}
}
