package residentagent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/transport"
)

type testPrivilegedClient struct {
	closes  atomic.Int32
	presses atomic.Int32
	err     error
	observe func(string)
}

func TestRenewableConnectionStillUsesBoundedInputLease(t *testing.T) {
	now := time.Now()
	client := &testPrivilegedClient{}
	var expires time.Time
	input := &privilegedLeaseInput{connect: func(_ context.Context, _ application.LeaseIdentity, end time.Time) (privilegedClient, error) {
		expires = end
		return client, nil
	}}
	if err := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, now); err != nil {
		t.Fatal(err)
	}
	defer input.close(nil)
	if !expires.Equal(now.Add(55 * time.Minute)) {
		t.Fatal("persistent consent made input lease unbounded", expires)
	}
}

func (p *testPrivilegedClient) SetTimingObserver(emit func(string)) { p.observe = emit }

func TestPrivilegedInputRoutesTimingThroughFixedEventSink(t *testing.T) {
	var events []string
	client := &testPrivilegedClient{}
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), event: func(code string) { events = append(events, code) }, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
		return client, nil
	}}
	if err := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now()); err != nil || client.observe == nil {
		t.Fatal("approved input omitted timing observer")
	}
	defer input.close(nil)
	client.observe("installed_input_timing_client_authority_lt1ms")
	if len(events) != 3 || events[0] != "installed_input_lease_ready" || !strings.HasPrefix(events[1], "installed_input_handoff_timing_admission_") || events[2] != "installed_input_timing_client_authority_lt1ms" {
		t.Fatal("diagnostic sink lost timing events or changed startup status")
	}
}

func TestPrivilegedInputFailedStartupLogsBoundedDiagnosticsWithoutGrant(t *testing.T) {
	var events []string
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), event: func(code string) { events = append(events, code) }, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
		return nil, errors.New("private diagnostic contents")
	}}
	if input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now()) != errPrivilegedInput || input.client != nil {
		t.Fatal("failed privileged startup granted an input channel")
	}
	if len(events) != 4 || events[0] != "installed_input_connect_failed" || events[1] != inputworker.ConnectionFailureEvent(nil) || events[2] != inputworker.ConnectionNativeFailureEvent(nil) || !strings.HasPrefix(events[3], "installed_input_handoff_timing_admission_") {
		t.Fatal("startup failure omitted bounded diagnostic events")
	}
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

func TestPrivilegedInputShutdownRetiresLeasesBeforeReleasingPins(t *testing.T) {
	first, second := &testPrivilegedClient{}, &testPrivilegedClient{}
	disposals, connects := 0, 0
	cause := errors.New("pin close failed")
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
		connects++
		if connects == 1 {
			return first, nil
		}
		return second, nil
	}}
	input.dispose = func() error {
		disposals++
		if second.closes.Load() != 1 || input.client != nil || input.done != nil || input.cancel != nil {
			t.Error("pins released before client and heartbeat retirement")
		}
		return cause
	}
	t.Cleanup(func() { _ = input.shutdown() })
	id := application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}
	if err := input.open(context.Background(), id, time.Now()); err != nil {
		t.Fatal(err)
	}
	input.close(nil)
	if first.closes.Load() != 1 || disposals != 0 {
		t.Fatal("boundary return released shared image pins")
	}
	if err := input.open(context.Background(), id, time.Now()); err != nil {
		t.Fatal("reentry failed", err)
	}
	if input.shutdown() != cause || input.shutdown() != cause || disposals != 1 {
		t.Fatal("cleanup failure hidden or pins closed more than once")
	}
	if input.open(context.Background(), id, time.Now()) != errPrivilegedInput || connects != 2 {
		t.Fatal("late lease reopened a retired installation")
	}
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
