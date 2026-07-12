package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestTransitionSupervisorCoversReconnectLifecycle(t *testing.T) {
	steps := []struct {
		event SupervisorEvent
		want  SupervisorState
	}{
		{SupervisorStarted, SupervisorAcquiring},
		{SupervisorConnectionAcquired, SupervisorActive},
		{SupervisorSessionFailed, SupervisorBackoff},
		{SupervisorRetryElapsed, SupervisorAcquiring},
		{SupervisorAcquireFailed, SupervisorBackoff},
		{SupervisorStoppedEvent, SupervisorStopped},
	}
	state := SupervisorStopped
	for _, step := range steps {
		transition, err := TransitionSupervisor(state, step.event)
		if err != nil {
			t.Fatalf("TransitionSupervisor(%s, %s) error = %v", state, step.event, err)
		}
		if transition.Next != step.want {
			t.Fatalf("TransitionSupervisor(%s, %s) = %s, want %s", state, step.event, transition.Next, step.want)
		}
		state = transition.Next
	}
}

func TestTransitionSupervisorRejectsImpossibleTransition(t *testing.T) {
	if _, err := TransitionSupervisor(SupervisorStopped, SupervisorSessionFailed); err == nil {
		t.Fatal("stopped supervisor accepted session failure")
	}
}

func TestBackoffPolicyDoublesAndCaps(t *testing.T) {
	policy := BackoffPolicy{Initial: 10 * time.Millisecond, Maximum: 40 * time.Millisecond, StableAfter: time.Second}
	wants := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 40 * time.Millisecond}
	for index, want := range wants {
		got, err := policy.Delay(uint32(index + 1))
		if err != nil || got != want {
			t.Fatalf("Delay(%d) = %s, %v; want %s", index+1, got, err, want)
		}
	}
}

func TestRunSupervisorRetriesAcquireAndRecoveredSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &scriptedSource{outcomes: []sourceOutcome{{err: errors.New("offline")}, {connection: &stubConnection{}}}}
	observer := &recordingSupervisorObserver{}
	waits := make([]time.Duration, 0, 2)
	config := SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now},
		Backoff: BackoffPolicy{Initial: 10 * time.Millisecond, Maximum: 40 * time.Millisecond, StableAfter: time.Second},
		Now:     time.Now,
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == 2 {
				cancel()
				return ctx.Err()
			}
			return nil
		},
		Observer: observer,
	}
	runnerCalls := 0
	err := RunSupervisor(ctx, config, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			runnerCalls++
			return Result{FinalState: control.StateLocal}, errors.New("connection lost")
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunSupervisor() error = %v", err)
	}
	if runnerCalls != 1 || len(waits) != 2 || waits[0] != 10*time.Millisecond || waits[1] != 20*time.Millisecond {
		t.Fatalf("runner=%d waits=%v", runnerCalls, waits)
	}
	wantStates := []SupervisorState{
		SupervisorAcquiring, SupervisorBackoff, SupervisorAcquiring,
		SupervisorActive, SupervisorBackoff, SupervisorStopped,
	}
	if got := observer.states(); !equalStates(got, wantStates) {
		t.Fatalf("states = %v, want %v", got, wantStates)
	}
	if !source.closed {
		t.Fatal("connection source was not closed")
	}
}

func TestRunSupervisorDoesNotReconnectUnsafeSession(t *testing.T) {
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}}
	observer := &recordingSupervisorObserver{}
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: Wait, Observer: observer,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateFault}, errors.New("cleanup incomplete")
		},
	})
	if err == nil || source.nextCalls != 1 {
		t.Fatalf("RunSupervisor() error = %v, Next calls = %d", err, source.nextCalls)
	}
	if got := observer.states(); !equalStates(got, []SupervisorState{SupervisorAcquiring, SupervisorActive, SupervisorStopped}) {
		t.Fatalf("states = %v", got)
	}
}

func TestRunSupervisorReportsStoppedAfterUnexpectedCleanSessionReturn(t *testing.T) {
	connection := &stubConnection{}
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: connection}}}
	observer := &recordingSupervisorObserver{}
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: Wait, Observer: observer,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, nil
		},
	})
	if err == nil {
		t.Fatal("RunSupervisor() accepted a session without failure or shutdown")
	}
	if !connection.closed || !source.closed {
		t.Fatalf("terminal resources remained open: connection=%t source=%t", connection.closed, source.closed)
	}
	if got := observer.states(); !equalStates(got, []SupervisorState{SupervisorAcquiring, SupervisorActive, SupervisorStopped}) {
		t.Fatalf("states = %v", got)
	}
}

func TestRunSupervisorClosesConnectionReturnedWithAcquireError(t *testing.T) {
	connection := &stubConnection{}
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: connection, err: errors.New("partial acquire")}}}
	observer := &recordingSupervisorObserver{}
	waitErr := errors.New("retry unavailable")
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now},
		Backoff: BackoffPolicy{Initial: time.Millisecond, Maximum: time.Millisecond, StableAfter: time.Second},
		Now:     time.Now, Wait: func(context.Context, time.Duration) error { return waitErr }, Observer: observer,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: RunSession,
	})
	if !errors.Is(err, waitErr) {
		t.Fatalf("RunSupervisor() error = %v, want retry error", err)
	}
	if !connection.closed || !source.closed {
		t.Fatalf("partial acquire resources remained open: connection=%t source=%t", connection.closed, source.closed)
	}
	if got := observer.states(); !equalStates(got, []SupervisorState{SupervisorAcquiring, SupervisorBackoff, SupervisorStopped}) {
		t.Fatalf("states = %v", got)
	}
}

func TestRunSupervisorDoesNotRetryWhenPartialAcquireCloseFails(t *testing.T) {
	acquireErr := errors.New("partial acquire")
	closeErr := errors.New("partial connection close failed")
	connection := &stubConnection{closeErr: closeErr}
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: connection, err: acquireErr}}}
	observer := &recordingSupervisorObserver{}
	waitCalled := false
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now,
		Wait: func(context.Context, time.Duration) error { waitCalled = true; return nil }, Observer: observer,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: RunSession,
	})
	if !errors.Is(err, acquireErr) || !errors.Is(err, closeErr) {
		t.Fatalf("RunSupervisor() error = %v, want acquire and close failures", err)
	}
	if waitCalled || source.nextCalls != 1 || connection.closeCalls != 1 {
		t.Fatalf("wait=%t Next=%d close=%d", waitCalled, source.nextCalls, connection.closeCalls)
	}
	if got := observer.states(); !equalStates(got, []SupervisorState{SupervisorAcquiring, SupervisorStopped}) {
		t.Fatalf("states = %v", got)
	}
}

func TestRunSupervisorPropagatesConnectionCloseFailureWithoutRetry(t *testing.T) {
	closeErr := errors.New("connection close failed")
	sessionErr := errors.New("session failed")
	connection := &stubConnection{closeErr: closeErr}
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: connection}}}
	observer := &recordingSupervisorObserver{}
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: Wait, Observer: observer,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(_ context.Context, _ Config, dependencies Dependencies) (Result, error) {
			_ = dependencies.Connection.Close()
			return Result{FinalState: control.StateLocal}, sessionErr
		},
	})
	if !errors.Is(err, sessionErr) || !errors.Is(err, closeErr) {
		t.Fatalf("RunSupervisor() error = %v, want session and close failures", err)
	}
	if connection.closeCalls != 1 || source.nextCalls != 1 {
		t.Fatalf("close calls = %d, Next calls = %d", connection.closeCalls, source.nextCalls)
	}
	if got := observer.states(); !equalStates(got, []SupervisorState{SupervisorAcquiring, SupervisorActive, SupervisorStopped}) {
		t.Fatalf("states = %v", got)
	}
}

func TestRunSupervisorPropagatesSourceCloseFailure(t *testing.T) {
	closeErr := errors.New("source close failed")
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}, closeErr: closeErr}
	observer := &recordingSupervisorObserver{}
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: Wait, Observer: observer,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, wininput.ErrEmergencyRelease
		},
	})
	if !errors.Is(err, wininput.ErrEmergencyRelease) || !errors.Is(err, closeErr) {
		t.Fatalf("RunSupervisor() error = %v, want emergency and source close failures", err)
	}
	if source.closeCalls != 1 {
		t.Fatalf("source close calls = %d", source.closeCalls)
	}
	if snapshots := observer.snapshotsCopy(); len(snapshots) == 0 || snapshots[len(snapshots)-1].State != SupervisorStopped || snapshots[len(snapshots)-1].FailureKind != "emergency_release" {
		t.Fatalf("snapshots = %+v", snapshots)
	}
}

func TestRunSupervisorStopsAfterEmergencyRelease(t *testing.T) {
	source := &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}}
	err := RunSupervisor(context.Background(), SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: Wait,
	}, SupervisorDependencies{
		Source: source, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, wininput.ErrEmergencyRelease
		},
	})
	if !errors.Is(err, wininput.ErrEmergencyRelease) || source.nextCalls != 1 {
		t.Fatalf("RunSupervisor() error = %v, Next calls = %d", err, source.nextCalls)
	}
}

type sourceOutcome struct {
	connection Connection
	err        error
}

type scriptedSource struct {
	outcomes   []sourceOutcome
	nextCalls  int
	closed     bool
	closeCalls int
	closeErr   error
}

func (source *scriptedSource) Next(context.Context) (Connection, error) {
	if source.nextCalls >= len(source.outcomes) {
		return nil, errors.New("no scripted outcome")
	}
	outcome := source.outcomes[source.nextCalls]
	source.nextCalls++
	return outcome.connection, outcome.err
}
func (source *scriptedSource) Close() error {
	source.closed = true
	source.closeCalls++
	return source.closeErr
}

type stubConnection struct {
	closed     bool
	closeCalls int
	closeErr   error
}

func (*stubConnection) AuthenticatedPeer() (transport.AuthenticatedPeerContext, error) {
	return testAuthenticatedPeerContext(), nil
}

func (*stubConnection) ReadFrame() (transport.Frame, error) {
	return transport.Frame{}, errors.New("unused")
}
func (*stubConnection) WriteFrameContext(context.Context, transport.Frame) error {
	return errors.New("unused")
}
func (connection *stubConnection) Close() error {
	connection.closed = true
	connection.closeCalls++
	return connection.closeErr
}

type recordingSupervisorObserver struct {
	mu        sync.Mutex
	snapshots []SupervisorSnapshot
}

func (observer *recordingSupervisorObserver) ObserveSupervisor(snapshot SupervisorSnapshot) {
	observer.mu.Lock()
	observer.snapshots = append(observer.snapshots, snapshot)
	observer.mu.Unlock()
}
func (observer *recordingSupervisorObserver) states() []SupervisorState {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	states := make([]SupervisorState, len(observer.snapshots))
	for index, snapshot := range observer.snapshots {
		states[index] = snapshot.State
	}
	return states
}
func (observer *recordingSupervisorObserver) snapshotsCopy() []SupervisorSnapshot {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return append([]SupervisorSnapshot(nil), observer.snapshots...)
}
func equalStates(left, right []SupervisorState) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
