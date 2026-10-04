package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/transport"
)

type preparationCapableConnection struct{ agent.Connection }

func (preparationCapableConnection) SupportsInputPreparation() bool { return true }

func TestProductDisconnectWrapperPreservesNegotiatedPreparation(t *testing.T) {
	for _, connection := range []agent.Connection{&disconnectTestConnection{}, preparationCapableConnection{}} {
		wrapped := &disconnectConnection{Connection: connection}
		_, expected := connection.(preparationCapableConnection)
		if wrapped.SupportsInputPreparation() != expected {
			t.Fatal("product wrapper changed preparation capability")
		}
	}
}

type testPreparationPolicy struct {
	testPrivilegedAuthorizer
	allowed bool
}

func (p testPreparationPolicy) AuthorizePreparation(context.Context, transport.AuthenticatedPeerContext, time.Time) (bool, error) {
	return p.allowed, p.err
}

func TestPreparationDoesNotOpenLeaseAndRejectsInactivePolicy(t *testing.T) {
	for _, test := range []struct {
		allowed, active, stopped bool
		err                      error
	}{
		{allowed: true}, {}, {allowed: true, err: errors.New("denied")},
		{allowed: true, active: true}, {allowed: true, stopped: true},
	} {
		calls := 0
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), stopped: test.stopped, prepare: func(context.Context) error { calls++; return nil }}
		if test.active {
			input.client = &testPrivilegedClient{}
		}
		a := privilegedAuthorizer{LeaseAuthorizer: testPreparationPolicy{testPrivilegedAuthorizer: testPrivilegedAuthorizer{err: test.err}, allowed: test.allowed}, input: input}
		a.PrepareInput(context.Background(), transport.AuthenticatedPeerContext{}, time.Now())
		want := 0
		if test.allowed && !test.active && !test.stopped && test.err == nil {
			want = 1
		}
		if calls != want || input.identity.SessionID != "" || input.cancel != nil || input.done != nil {
			t.Fatal("preparation bypassed policy or established a lease")
		}
	}
}

func TestPreparationLockOverlapClipsOnlyTheLatestInterval(t *testing.T) {
	base := time.Unix(100, 0)
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	for _, tc := range []struct {
		start, end, prepStart, prepEnd int
		want                           time.Duration
	}{
		{0, 10, 2, 8, 6 * time.Millisecond},
		{4, 10, 2, 8, 4 * time.Millisecond},
		{0, 5, 2, 8, 3 * time.Millisecond},
		{0, 10, -2, 12, 10 * time.Millisecond},
		{0, 10, -8, -2, 0},
		{0, 10, 12, 18, 0},
		{10, 0, 2, 8, 0},
		{0, 10, 8, 2, 0},
	} {
		if got := preparationLockOverlap(at(tc.start), at(tc.end), at(tc.prepStart), at(tc.prepEnd)); got != tc.want {
			t.Fatal("overlap includes unrelated wait or preparation time", got, tc.want)
		}
	}
	if preparationLockOverlap(base, at(10), time.Time{}, time.Time{}) != 0 {
		t.Fatal("missing preparation became an overlap")
	}
}

func TestPreparationCompletionStillFencesShutdownAndPinDisposal(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	prepared, stopped := make(chan struct{}), make(chan struct{})
	disposed := make(chan struct{})
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), prepare: func(context.Context) error {
		close(entered)
		<-release
		return nil
	}, dispose: func() error { close(disposed); return nil }}
	a := privilegedAuthorizer{LeaseAuthorizer: testPreparationPolicy{allowed: true}, input: input}
	go func() {
		a.PrepareInput(context.Background(), transport.AuthenticatedPeerContext{}, time.Now())
		close(prepared)
	}()
	<-entered
	if input.mu.TryLock() {
		input.mu.Unlock()
		close(release)
		t.Fatal("preparation no longer retains the shutdown fence")
	}
	go func() { input.shutdown(); close(stopped) }()
	select {
	case <-disposed:
		close(release)
		t.Fatal("installation pins disposed during preparation")
	default:
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown failed to join preparation")
	}
	<-prepared
	<-disposed
	input.mu.Lock()
	valid := !input.preparationStarted.IsZero() && !input.preparationFinished.Before(input.preparationStarted) && input.stopped
	input.mu.Unlock()
	if !valid {
		t.Fatal("completed lock interval or stop state lost")
	}
}
