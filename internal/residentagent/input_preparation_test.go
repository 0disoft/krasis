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
