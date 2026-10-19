package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

func TestManagedPeerSessionRevokesExactInboundLeaseAndNotifiesPeer(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	connection := &managementTestConnection{}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, nil,
		func() time.Time { return time.Unix(200, 0) }, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	request := managementTestRequest("request-inbound", LeaseRevocationOnly)
	if err := managed.Revoke(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime state = %s", snapshot.Control.State)
	}
	if source.Active() || source.Deactivations() != 1 {
		t.Fatalf("source active = %v, deactivations = %d", source.Active(), source.Deactivations())
	}
	frames, interrupts := connection.Snapshot()
	if len(frames) != 1 || len(interrupts) != 0 {
		t.Fatalf("frames = %d, interrupts = %d", len(frames), len(interrupts))
	}
	command, err := transport.DecodeControlPayload(frames[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if command.Kind != transport.ControlLeaseRevoke ||
		command.SessionID != request.Lease.SessionID ||
		command.LeaseID != request.Lease.LeaseID ||
		command.Epoch != request.Lease.Epoch {
		t.Fatalf("revoke command = %+v", command)
	}
}

func TestManagedPeerSessionRejectsDifferentAuthenticatedPeer(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	connection := &managementTestConnection{}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, nil, time.Now, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	request := managementTestRequest("request-peer", LeaseRevocationOnly)
	request.Peer.CertificateSHA256 = strings.Repeat("b", 64)
	if err := managed.Revoke(context.Background(), request); !errors.Is(err, ErrManagementPeerMismatch) {
		t.Fatalf("Revoke() error = %v", err)
	}
	if source.Deactivations() != 0 || runtime.Snapshot().Control.State != control.StateInboundActive {
		t.Fatalf("different peer changed active session")
	}
	frames, interrupts := connection.Snapshot()
	if len(frames) != 0 || len(interrupts) != 0 {
		t.Fatalf("frames = %d, interrupts = %d", len(frames), len(interrupts))
	}
}

func TestManagedPeerSessionRejectsStaleRequestWithoutSideEffects(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	connection := &managementTestConnection{}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, nil, time.Now, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	request := managementTestRequest("request-stale", LeaseRevocationOnly)
	request.Lease.LeaseID = "stale-lease"
	if err := managed.Revoke(context.Background(), request); !errors.Is(err, ErrManagementLeaseMismatch) {
		t.Fatalf("Revoke() error = %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive {
		t.Fatalf("runtime state = %s", snapshot.Control.State)
	}
	if !source.Active() || source.Deactivations() != 0 {
		t.Fatalf("source active = %v, deactivations = %d", source.Active(), source.Deactivations())
	}
	frames, interrupts := connection.Snapshot()
	if len(frames) != 0 || len(interrupts) != 0 {
		t.Fatalf("frames = %d, interrupts = %d", len(frames), len(interrupts))
	}
}

func TestManagedPeerSessionClosesConnectionWhenPeerNotificationFails(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	connection := &managementTestConnection{writeErr: errors.New("network unavailable")}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, nil, time.Now, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := managed.Revoke(
		context.Background(),
		managementTestRequest("request-network", LeaseRevocationOnly),
	); !errors.Is(err, ErrManagementPeerNotification) {
		t.Fatalf("Revoke() error = %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime state = %s", snapshot.Control.State)
	}
	_, interrupts := connection.Snapshot()
	if len(interrupts) != 1 || !errors.Is(interrupts[0], ErrManagementPeerNotification) {
		t.Fatalf("interrupts = %v", interrupts)
	}
}

func TestManagedPeerSessionRevokeTrustIsIdempotentAndDisconnects(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	connection := &managementTestConnection{}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	trust := &managementTestTrustRevoker{}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, trust,
		func() time.Time { return time.Unix(300, 0) }, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewDefaultSessionRegistry()
	registration, err := registry.register(managed)
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Close()
	principal := managementTestPrincipal(t)
	first := managementTestRequest("request-trust-1", LeaseRevocationTrust)
	second := managementTestRequest("request-trust-2", LeaseRevocationTrust)
	type outcome struct {
		result LeaseRevocationResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	go func() {
		result, revokeErr := registry.Revoke(context.Background(), principal, first)
		outcomes <- outcome{result, revokeErr}
	}()
	go func() {
		result, revokeErr := registry.Revoke(context.Background(), principal, second)
		outcomes <- outcome{result, revokeErr}
	}()
	left := <-outcomes
	right := <-outcomes
	completed := 0
	stale := 0
	for _, current := range []outcome{left, right} {
		switch current.result.Code {
		case ManagementCodeCompleted:
			completed++
		case ManagementCodeLeaseMismatch:
			stale++
		default:
			t.Fatalf("unexpected result = %+v, error = %v", current.result, current.err)
		}
	}
	if completed != 1 || stale != 1 || trust.Calls() != 1 {
		t.Fatalf("completed = %d, stale = %d, trust calls = %d", completed, stale, trust.Calls())
	}
	_, interrupts := connection.Snapshot()
	if len(interrupts) != 1 || !errors.Is(interrupts[0], ErrManagementDisconnect) {
		t.Fatalf("interrupts = %v", interrupts)
	}
}

func TestManagedPeerSessionSerializesWithInputAndHeartbeatCoordinator(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	connection := &managementTestConnection{}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, nil, time.Now, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- coordinator.Execute(context.Background(), func(peer.FrameWriter) error {
			close(acquired)
			<-release
			return nil
		})
	}()
	<-acquired
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := managed.Revoke(
		ctx,
		managementTestRequest("request-serialized", LeaseRevocationOnly),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Revoke() error = %v", err)
	}
	if source.Deactivations() != 0 || runtime.Snapshot().Control.State != control.StateInboundActive {
		t.Fatalf("revoke crossed occupied coordinator")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestManagedPeerSessionDoesNotCancelLeaseAfterLocalEmergencyRelease(t *testing.T) {
	runtime := managementTestInboundRuntime(t)
	request := managementTestRequest("request-emergency", LeaseRevocationOnly)
	if _, err := runtime.RevokePeer(
		context.Background(), request.Lease.SessionID, request.Lease.LeaseID, request.Lease.Epoch,
	); err != nil {
		t.Fatal(err)
	}
	connection := &managementTestConnection{}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		t.Fatal(err)
	}
	source := &managementTestSource{active: true}
	managed, err := newManagedPeerSession(
		managementTestPeer(), runtime, coordinator, source, connection, nil, time.Now, newTestInput(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := managed.Revoke(context.Background(), request); !errors.Is(err, ErrManagementLeaseMismatch) {
		t.Fatalf("Revoke() error = %v", err)
	}
	if source.Deactivations() != 0 {
		t.Fatalf("source deactivations = %d", source.Deactivations())
	}
	frames, interrupts := connection.Snapshot()
	if len(frames) != 0 || len(interrupts) != 0 {
		t.Fatalf("frames = %d, interrupts = %d", len(frames), len(interrupts))
	}
}

func managementTestInboundRuntime(t *testing.T) *application.SessionRuntime {
	t.Helper()
	runtime, err := application.NewSessionRuntime(managementTestEffects{}, &managementTestInput{})
	if err != nil {
		t.Fatal(err)
	}
	request := managementTestRequest("setup", LeaseRevocationOnly)
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind:      control.EventInboundLeaseGranted,
		SessionID: request.Lease.SessionID,
		LeaseID:   request.Lease.LeaseID,
	}, request.Lease.Epoch, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	return runtime
}

type managementTestEffects struct{}

func (managementTestEffects) Execute(context.Context, control.Effect) error { return nil }

type managementTestInput struct{}

func (*managementTestInput) Press(context.Context, control.PressedToken) error   { return nil }
func (*managementTestInput) Release(context.Context, control.PressedToken) error { return nil }
func (*managementTestInput) Move(context.Context, int32, int32) error            { return nil }
func (*managementTestInput) Wheel(context.Context, int32, int32) error           { return nil }

type managementTestSource struct {
	mu            sync.Mutex
	active        bool
	deactivations int
}

func (source *managementTestSource) Activate() error {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.active = true
	return nil
}

func (source *managementTestSource) Deactivate() {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.active = false
	source.deactivations++
}

func (source *managementTestSource) Active() bool {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.active
}

func (source *managementTestSource) Deactivations() int {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.deactivations
}

type managementTestConnection struct {
	mu           sync.Mutex
	frames       []transport.Frame
	interrupts   []error
	writeErr     error
	interruptErr error
}

func (connection *managementTestConnection) WriteFrameContext(_ context.Context, frame transport.Frame) error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.writeErr != nil {
		return connection.writeErr
	}
	connection.frames = append(connection.frames, frame)
	return nil
}

func (connection *managementTestConnection) InterruptLocal(cause error) error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.interrupts = append(connection.interrupts, cause)
	return connection.interruptErr
}

func (connection *managementTestConnection) Snapshot() ([]transport.Frame, []error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return append([]transport.Frame(nil), connection.frames...), append([]error(nil), connection.interrupts...)
}

type managementTestTrustRevoker struct {
	calls atomic.Uint64
	err   error
}

func (revoker *managementTestTrustRevoker) RevokeTrust(string, time.Time) error {
	revoker.calls.Add(1)
	return revoker.err
}

func (revoker *managementTestTrustRevoker) Calls() uint64 {
	return revoker.calls.Load()
}
