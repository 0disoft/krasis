package peer

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPeerSessionsCompleteOutboundInputAndReturnRevokeRoundTrip(t *testing.T) {
	sourceSide, destinationSide := openFramedPipe(t)
	sourceInput := &integrationInputPort{moves: make(chan [2]int32, 1)}
	destinationInput := &integrationInputPort{moves: make(chan [2]int32, 1)}
	sourceRuntime := newIntegrationRuntime(t, sourceInput)
	destinationRuntime := newIntegrationRuntime(t, destinationInput)
	sourceActivation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(sourceSide, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	returnBoundary := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Writer:     destinationSide,
		Runtime:    destinationRuntime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Entry:      integrationCursorEntry{},
		Returns:    returnBoundary,
	}
	returnController, err := NewReturnController(destinationSide, destinationRuntime, returnBoundary)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	now := func() time.Time { return time.Unix(100, 0) }
	go func() { done <- RunSessionWithLifecycle(ctx, sourceSide, sourceRuntime, nil, outbound, now) }()
	go func() { done <- RunSessionWithLifecycle(ctx, destinationSide, destinationRuntime, responder, nil, now) }()
	t.Cleanup(func() {
		cancel()
		_ = sourceSide.Close()
		_ = destinationSide.Close()
		for range 2 {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("peer session did not stop")
			}
		}
	})

	identity := OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}
	if err := outbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		identity,
	); err != nil {
		t.Fatalf("BeginBoundary() error = %v", err)
	}
	waitSignal(t, sourceActivation.activated, "source activation after grant")
	if sourceRuntime.Snapshot().Control.State != control.StateOutboundActive {
		t.Fatalf("source runtime = %s", sourceRuntime.Snapshot())
	}
	if destinationRuntime.Snapshot().Control.State != control.StateInboundActive {
		t.Fatalf("destination runtime = %s", destinationRuntime.Snapshot())
	}

	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 12, DeltaY: -4}
	if err := SendInputAction(ctx, sourceSide, sourceRuntime, action); err != nil {
		t.Fatalf("SendInputAction() error = %v", err)
	}
	select {
	case move := <-destinationInput.moves:
		if move != [2]int32{12, -4} {
			t.Fatalf("destination move = %v", move)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("destination did not inject input")
	}

	if err := returnController.Revoke(ctx, control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
		t.Fatalf("return Revoke() error = %v", err)
	}
	waitSignal(t, sourceActivation.deactivated, "source deactivation after return revoke")
	if sourceRuntime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("source did not converge local: %s", sourceRuntime.Snapshot())
	}
	if destinationRuntime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("destination did not converge local: %s", destinationRuntime.Snapshot())
	}
}

func TestPeerSessionsReconnectWithNewSessionAndEpoch(t *testing.T) {
	sourceRuntime := newIntegrationRuntime(t, &integrationInputPort{moves: make(chan [2]int32, 2)})
	destinationInput := &integrationInputPort{moves: make(chan [2]int32, 2)}
	destinationRuntime := newIntegrationRuntime(t, destinationInput)
	completeIntegrationRoundTrip(
		t,
		sourceRuntime,
		destinationRuntime,
		destinationInput,
		OutboundIdentity{SessionID: "session-old", Epoch: "epoch-old"},
		[2]int32{3, 1},
	)
	completeIntegrationRoundTrip(
		t,
		sourceRuntime,
		destinationRuntime,
		destinationInput,
		OutboundIdentity{SessionID: "session-new", Epoch: "epoch-new"},
		[2]int32{-5, 2},
	)
	if sourceRuntime.Snapshot().Control.State != control.StateLocal || destinationRuntime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("reconnected runtimes did not finish local: source=%s destination=%s", sourceRuntime.Snapshot(), destinationRuntime.Snapshot())
	}
}

func TestPeerSessionsRejectStaleInputAfterReconnect(t *testing.T) {
	sourceRuntime := newIntegrationRuntime(t, &integrationInputPort{moves: make(chan [2]int32, 2)})
	destinationInput := &integrationInputPort{moves: make(chan [2]int32, 2)}
	destinationRuntime := newIntegrationRuntime(t, destinationInput)
	oldIdentity := OutboundIdentity{SessionID: "session-old", Epoch: "epoch-old"}
	completeIntegrationRoundTrip(t, sourceRuntime, destinationRuntime, destinationInput, oldIdentity, [2]int32{2, 1})

	sourceSide, destinationSide := openFramedPipe(t)
	sourceActivation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(sourceSide, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	returnBoundary := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Writer:     destinationSide,
		Runtime:    destinationRuntime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-new", accepted: true},
		Entry:      integrationCursorEntry{},
		Returns:    returnBoundary,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { done <- RunSessionWithLifecycle(ctx, sourceSide, sourceRuntime, nil, outbound, time.Now) }()
	go func() {
		done <- RunSessionWithLifecycle(ctx, destinationSide, destinationRuntime, responder, nil, time.Now)
	}()
	newIdentity := OutboundIdentity{SessionID: "session-new", Epoch: "epoch-new"}
	if err := outbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		newIdentity,
	); err != nil {
		t.Fatalf("BeginBoundary() error = %v", err)
	}
	waitSignal(t, sourceActivation.activated, "reconnected source activation")
	payload, err := transport.EncodeInputPayload(
		control.InputEnvelope{SessionID: oldIdentity.SessionID, Epoch: oldIdentity.Epoch, Sequence: 1},
		control.InputAction{Kind: control.InputPointerMove, DeltaX: 99},
	)
	if err != nil {
		t.Fatalf("EncodeInputPayload(stale) error = %v", err)
	}
	if err := sourceSide.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageInput, Payload: payload}); err != nil {
		t.Fatalf("WriteFrameContext(stale) error = %v", err)
	}
	waitSignal(t, sourceActivation.deactivated, "source cleanup after stale input")
	if sourceRuntime.Snapshot().Control.State != control.StateLocal || destinationRuntime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("stale input did not fail safe: source=%s destination=%s", sourceRuntime.Snapshot(), destinationRuntime.Snapshot())
	}
	select {
	case move := <-destinationInput.moves:
		t.Fatalf("stale input was injected: %v", move)
	default:
	}
	cancel()
	_ = sourceSide.Close()
	_ = destinationSide.Close()
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("stale-input peer session did not stop")
		}
	}
}

func completeIntegrationRoundTrip(
	t *testing.T,
	sourceRuntime *application.SessionRuntime,
	destinationRuntime *application.SessionRuntime,
	destinationInput *integrationInputPort,
	identity OutboundIdentity,
	move [2]int32,
) {
	t.Helper()
	sourceSide, destinationSide := openFramedPipe(t)
	sourceActivation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(sourceSide, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	returns := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Writer:     destinationSide,
		Runtime:    destinationRuntime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-" + identity.SessionID, accepted: true},
		Entry:      integrationCursorEntry{},
		Returns:    returns,
	}
	returnController, err := NewReturnController(destinationSide, destinationRuntime, returns)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { done <- RunSessionWithLifecycle(ctx, sourceSide, sourceRuntime, nil, outbound, time.Now) }()
	go func() {
		done <- RunSessionWithLifecycle(ctx, destinationSide, destinationRuntime, responder, nil, time.Now)
	}()
	if err := outbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		identity,
	); err != nil {
		t.Fatalf("BeginBoundary(%s) error = %v", identity.SessionID, err)
	}
	waitSignal(t, sourceActivation.activated, "round-trip source activation")
	if err := SendInputAction(ctx, sourceSide, sourceRuntime, control.InputAction{Kind: control.InputPointerMove, DeltaX: move[0], DeltaY: move[1]}); err != nil {
		t.Fatalf("SendInputAction(%s) error = %v", identity.SessionID, err)
	}
	select {
	case got := <-destinationInput.moves:
		if got != move {
			t.Fatalf("destination move = %v, want %v", got, move)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("destination did not receive %s input", identity.SessionID)
	}
	if err := returnController.Revoke(ctx, control.ReturnIntent{SessionID: identity.SessionID, Epoch: identity.Epoch}); err != nil {
		t.Fatalf("Revoke(%s) error = %v", identity.SessionID, err)
	}
	waitSignal(t, sourceActivation.deactivated, "round-trip source deactivation")
	if sourceRuntime.Snapshot().Control.State != control.StateLocal || destinationRuntime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("round trip did not finish local: source=%s destination=%s", sourceRuntime.Snapshot(), destinationRuntime.Snapshot())
	}
	cancel()
	_ = sourceSide.Close()
	_ = destinationSide.Close()
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("round-trip peer session did not stop")
		}
	}
}

func newIntegrationRuntime(t *testing.T, input application.InputPort) *application.SessionRuntime {
	t.Helper()
	runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	return runtime
}

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

type integrationInputPort struct {
	moves    chan [2]int32
	presses  chan control.PressedToken
	releases chan control.PressedToken
}

func (port *integrationInputPort) Press(_ context.Context, token control.PressedToken) error {
	if port.presses != nil {
		port.presses <- token
	}
	return nil
}
func (port *integrationInputPort) Release(_ context.Context, token control.PressedToken) error {
	if port.releases != nil {
		port.releases <- token
	}
	return nil
}
func (port *integrationInputPort) Move(_ context.Context, deltaX, deltaY int32) error {
	port.moves <- [2]int32{deltaX, deltaY}
	return nil
}
func (*integrationInputPort) Wheel(context.Context, int32, int32) error { return nil }

type integrationSourceActivation struct {
	mu          sync.Mutex
	active      bool
	activated   chan struct{}
	deactivated chan struct{}
}

func newIntegrationSourceActivation() *integrationSourceActivation {
	return &integrationSourceActivation{
		activated:   make(chan struct{}, 1),
		deactivated: make(chan struct{}, 1),
	}
}

func (source *integrationSourceActivation) Activate() error {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.active {
		return errors.New("source already active")
	}
	source.active = true
	source.activated <- struct{}{}
	return nil
}

func (source *integrationSourceActivation) Deactivate() {
	source.mu.Lock()
	wasActive := source.active
	source.active = false
	source.mu.Unlock()
	if wasActive {
		select {
		case source.deactivated <- struct{}{}:
		default:
		}
	}
}

func (source *integrationSourceActivation) Active() bool {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.active
}

type integrationCursorEntry struct{}

func (integrationCursorEntry) Enter(context.Context, control.ScreenEdge) error { return nil }

type integrationReturnBoundary struct {
	mu     sync.Mutex
	intent control.ReturnIntent
}

func (boundary *integrationReturnBoundary) ArmReturn(intent control.ReturnIntent, _ control.ScreenEdge) error {
	boundary.mu.Lock()
	boundary.intent = intent
	boundary.mu.Unlock()
	return nil
}

func (boundary *integrationReturnBoundary) RearmReturn(control.ReturnIntent) {}

func (boundary *integrationReturnBoundary) DisarmReturn(intent control.ReturnIntent) {
	boundary.mu.Lock()
	if boundary.intent == intent {
		boundary.intent = control.ReturnIntent{}
	}
	boundary.mu.Unlock()
}

type framedPipeConnection struct {
	connection net.Conn
	reader     *transport.Reader
	writer     *transport.Writer
	writeMu    sync.Mutex
}

func openFramedPipe(t *testing.T) (*framedPipeConnection, *framedPipeConnection) {
	t.Helper()
	left, right := net.Pipe()
	newSide := func(connection net.Conn) *framedPipeConnection {
		reader, err := transport.NewReader(connection)
		if err != nil {
			t.Fatalf("NewReader() error = %v", err)
		}
		writer, err := transport.NewWriter(connection)
		if err != nil {
			t.Fatalf("NewWriter() error = %v", err)
		}
		return &framedPipeConnection{connection: connection, reader: reader, writer: writer}
	}
	return newSide(left), newSide(right)
}

func (connection *framedPipeConnection) ReadFrame() (transport.Frame, error) {
	return connection.reader.ReadFrame()
}

func (connection *framedPipeConnection) WriteFrameContext(ctx context.Context, frame transport.Frame) error {
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		if err := connection.connection.SetWriteDeadline(deadline); err != nil {
			return err
		}
		defer connection.connection.SetWriteDeadline(time.Time{})
	}
	return connection.writer.WriteFrame(frame)
}

func (connection *framedPipeConnection) Close() error {
	return connection.connection.Close()
}
