package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type diagnosticStubConnection struct {
	stubConnection
	id string
}

func (connection *diagnosticStubConnection) DiagnosticID() string { return connection.id }

func TestSupervisorKeepsFailureCorrelationAndClearsItBeforeNewAcquisition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observer := &recordingSupervisorObserver{}
	waits := 0
	err := RunSupervisor(ctx, SupervisorConfig{Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Observer: observer, Wait: func(context.Context, time.Duration) error {
		waits++
		if waits == 2 {
			cancel()
			return context.Canceled
		}
		return nil
	}}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &diagnosticStubConnection{id: "connection-one"}}, {err: errors.New("offline")}}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, errors.New("peer failure")
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	seen := 0
	for _, snapshot := range observer.snapshotsCopy() {
		want := ""
		if snapshot.State == SupervisorActive || snapshot.FailureKind == "session" {
			want = "connection-one"
			seen++
		}
		if snapshot.ConnectionID != want {
			t.Fatalf("stale or lost correlation: %+v", snapshot)
		}
	}
	if seen != 2 {
		t.Fatalf("missing active/failure snapshots: %d", seen)
	}
}
