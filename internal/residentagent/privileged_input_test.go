package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type testPrivilegedClient struct {
	closes  atomic.Int32
	presses atomic.Int32
	err     error
}

func (p *testPrivilegedClient) Press(context.Context, control.PressedToken) error {
	p.presses.Add(1)
	return p.err
}
func (p *testPrivilegedClient) Release(context.Context, control.PressedToken) error { return p.err }
func (p *testPrivilegedClient) Move(context.Context, int32, int32) error            { return p.err }
func (p *testPrivilegedClient) Wheel(context.Context, int32, int32) error           { return p.err }
func (p *testPrivilegedClient) Heartbeat(context.Context) error                     { return p.err }
func (p *testPrivilegedClient) Close() error                                        { p.closes.Add(1); return nil }

type testPrivilegedAuthorizer struct {
	accepted bool
	err      error
}

func (a testPrivilegedAuthorizer) AuthorizeLease(context.Context, transport.AuthenticatedPeerContext, string, string, time.Time) (string, bool, error) {
	return "lease", a.accepted, a.err
}

func TestPrivilegedInputRequiresApprovalAndExactLeaseRetirement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &testPrivilegedClient{}
	connects := 0
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), connect: func(ctx context.Context, id application.LeaseIdentity, expiry time.Time) (privilegedClient, error) {
		connects++
		if id != (application.LeaseIdentity{SessionID: "s", LeaseID: "lease", Epoch: "e"}) || !expiry.After(time.Now()) || expiry.After(time.Now().Add(time.Hour)) {
			t.Fatal("unbound worker lease")
		}
		return p, nil
	}}
	a := privilegedAuthorizer{LeaseAuthorizer: testPrivilegedAuthorizer{}, input: input}
	if _, accepted, err := a.AuthorizeLease(ctx, transport.AuthenticatedPeerContext{}, "s", "e", time.Now()); err != nil || accepted || connects != 0 {
		t.Fatal("rejected peer opened input worker")
	}
	a.LeaseAuthorizer = testPrivilegedAuthorizer{accepted: true}
	if id, accepted, err := a.AuthorizeLease(ctx, transport.AuthenticatedPeerContext{}, "s", "e", time.Now()); err != nil || !accepted || id != "lease" || connects != 1 {
		t.Fatal("approved lease did not open worker")
	}
	if _, _, err := a.AuthorizeLease(ctx, transport.AuthenticatedPeerContext{}, "s", "e", time.Now()); err == nil || connects != 1 {
		t.Fatal("active worker overwritten")
	}
	input.close(&control.ReturnIntent{SessionID: "old", Epoch: "e"})
	if p.closes.Load() != 0 {
		t.Fatal("stale return closed current worker")
	}
	input.close(&control.ReturnIntent{SessionID: "s", Epoch: "e"})
	input.close(nil)
	if p.closes.Load() != 1 {
		t.Fatal("worker cleanup not idempotent")
	}
	if err := input.Press(ctx, control.PressedToken{}); err == nil {
		t.Fatal("input accepted without lease")
	}
}

func TestPrivilegedInputPreservesUnknownOutcomeWithoutReplay(t *testing.T) {
	p := &testPrivilegedClient{err: &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}}
	input := &privilegedLeaseInput{client: p}
	if err := input.Press(context.Background(), control.PressedToken{}); !errors.Is(err, p.err) || p.presses.Load() != 1 {
		t.Fatal("uncertain press replayed or lost")
	}
	input.close(nil)
}

func TestPrivilegedWorkerHeartbeatFailureCancelsSession(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	p := &testPrivilegedClient{err: errors.New("lost channel")}
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), failed: cancel, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) { return p, nil }}
	defer input.close(nil)
	if err := input.open(ctx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), errPrivilegedInput) {
			t.Fatal("lost worker hidden")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle worker failure did not stop session")
	}
}
