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
	sourceCoordinator := testWriteCoordinator(t, sourceSide)
	destinationCoordinator := testWriteCoordinator(t, destinationSide)
	sourceActivation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(sourceCoordinator, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	returnBoundary := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Coordinator: destinationCoordinator,
		Runtime:     destinationRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     returnBoundary,
	}
	returnController, err := NewReturnController(destinationCoordinator, destinationRuntime, returnBoundary)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	now := func() time.Time { return time.Unix(100, 0) }
	go func() {
		done <- RunSessionWithLifecycle(ctx, sourceSide, sourceCoordinator, sourceRuntime, nil, outbound, now)
	}()
	go func() {
		done <- RunSessionWithLifecycle(ctx, destinationSide, destinationCoordinator, destinationRuntime, responder, nil, now)
	}()
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
	waitRuntimeState(t, destinationRuntime, control.StateInboundActive, "destination commit")
	if sourceRuntime.Snapshot().Control.State != control.StateOutboundActive {
		t.Fatalf("source runtime = %s", sourceRuntime.Snapshot())
	}
	if destinationRuntime.Snapshot().Control.State != control.StateInboundActive {
		t.Fatalf("destination runtime = %s", destinationRuntime.Snapshot())
	}

	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 12, DeltaY: -4}
	if err := SendInputAction(ctx, sourceCoordinator, sourceRuntime, action); err != nil {
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

func TestPeerSessionsAbortPreparedDestinationWhenSourceActivationFails(t *testing.T) {
	sourceSide, destinationSide := openFramedPipe(t)
	sourceRuntime := newIntegrationRuntime(t, &integrationInputPort{})
	destinationRuntime := newIntegrationRuntime(t, &integrationInputPort{})
	sourceCoordinator := testWriteCoordinator(t, sourceSide)
	destinationCoordinator := testWriteCoordinator(t, destinationSide)
	sourceActivation := newIntegrationSourceActivation()
	sourceActivation.activateErr = errors.New("foreground activation denied")
	outbound, err := NewOutboundController(sourceCoordinator, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatal(err)
	}
	rolledBack := make(chan struct{}, 1)
	entry := integrationCursorEntry{rolledBack: rolledBack}
	returns := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Coordinator: destinationCoordinator, Runtime: destinationRuntime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:       testAuthenticatedPeer(), Entry: entry, Returns: returns,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	now := func() time.Time { return time.Unix(100, 0) }
	go func() {
		done <- RunSessionWithLifecycle(ctx, sourceSide, sourceCoordinator, sourceRuntime, nil, outbound, now)
	}()
	go func() {
		done <- RunSessionWithLifecycle(ctx, destinationSide, destinationCoordinator, destinationRuntime, responder, nil, now)
	}()
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

	if err := outbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"},
	); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, sourceActivation.attempted, "source activation attempt")
	waitSignal(t, rolledBack, "destination prepared-entry rollback")
	waitRuntimeState(t, sourceRuntime, control.StateLocal, "source abort convergence")
	waitRuntimeState(t, destinationRuntime, control.StateLocal, "destination abort convergence")
}

func TestPeerSessionsReuseOneConnectionForBothDirections(t *testing.T) {
	leftSide, rightSide := openFramedPipe(t)
	leftInput := &integrationInputPort{moves: make(chan [2]int32, 1)}
	rightInput := &integrationInputPort{moves: make(chan [2]int32, 1)}
	leftRuntime := newIntegrationRuntime(t, leftInput)
	rightRuntime := newIntegrationRuntime(t, rightInput)
	leftCoordinator := testWriteCoordinator(t, leftSide)
	rightCoordinator := testWriteCoordinator(t, rightSide)
	leftActivation := newIntegrationSourceActivation()
	rightActivation := newIntegrationSourceActivation()
	leftOutbound, err := NewOutboundController(leftCoordinator, leftRuntime, leftActivation)
	if err != nil {
		t.Fatalf("NewOutboundController(left) error = %v", err)
	}
	rightOutbound, err := NewOutboundController(rightCoordinator, rightRuntime, rightActivation)
	if err != nil {
		t.Fatalf("NewOutboundController(right) error = %v", err)
	}
	leftReturns := &integrationReturnBoundary{}
	rightReturns := &integrationReturnBoundary{}
	leftResponder := &LeaseResponder{
		Coordinator: leftCoordinator,
		Runtime:     leftRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-left", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     leftReturns,
	}
	rightResponder := &LeaseResponder{
		Coordinator: rightCoordinator,
		Runtime:     rightRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-right", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     rightReturns,
	}
	leftReturnController, err := NewReturnController(leftCoordinator, leftRuntime, leftReturns)
	if err != nil {
		t.Fatalf("NewReturnController(left) error = %v", err)
	}
	rightReturnController, err := NewReturnController(rightCoordinator, rightRuntime, rightReturns)
	if err != nil {
		t.Fatalf("NewReturnController(right) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() {
		done <- RunSessionWithLifecycle(ctx, leftSide, leftCoordinator, leftRuntime, leftResponder, leftOutbound, time.Now)
	}()
	go func() {
		done <- RunSessionWithLifecycle(ctx, rightSide, rightCoordinator, rightRuntime, rightResponder, rightOutbound, time.Now)
	}()
	t.Cleanup(func() {
		cancel()
		_ = leftSide.Close()
		_ = rightSide.Close()
		for range 2 {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("bidirectional peer session did not stop")
			}
		}
	})

	leftIdentity := OutboundIdentity{SessionID: "session-left", Epoch: "epoch-left"}
	if err := leftOutbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		leftIdentity,
	); err != nil {
		t.Fatalf("BeginBoundary(left) error = %v", err)
	}
	waitSignal(t, leftActivation.activated, "left source activation")
	if err := SendInputAction(ctx, leftCoordinator, leftRuntime, control.InputAction{Kind: control.InputPointerMove, DeltaX: 7, DeltaY: -2}); err != nil {
		t.Fatalf("SendInputAction(left) error = %v", err)
	}
	waitMove(t, rightInput.moves, [2]int32{7, -2}, "right")
	if err := rightReturnController.Revoke(ctx, control.ReturnIntent{SessionID: leftIdentity.SessionID, Epoch: leftIdentity.Epoch}); err != nil {
		t.Fatalf("Revoke(right) error = %v", err)
	}
	waitSignal(t, leftActivation.deactivated, "left source deactivation")
	assertIntegrationRuntimesLocal(t, leftRuntime, rightRuntime, "left-to-right transfer")

	rightIdentity := OutboundIdentity{SessionID: "session-right", Epoch: "epoch-right"}
	if err := rightOutbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeLeft, X: 0},
		rightIdentity,
	); err != nil {
		t.Fatalf("BeginBoundary(right) error = %v", err)
	}
	waitSignal(t, rightActivation.activated, "right source activation")
	if err := SendInputAction(ctx, rightCoordinator, rightRuntime, control.InputAction{Kind: control.InputPointerMove, DeltaX: -9, DeltaY: 3}); err != nil {
		t.Fatalf("SendInputAction(right) error = %v", err)
	}
	waitMove(t, leftInput.moves, [2]int32{-9, 3}, "left")
	if err := leftReturnController.Revoke(ctx, control.ReturnIntent{SessionID: rightIdentity.SessionID, Epoch: rightIdentity.Epoch}); err != nil {
		t.Fatalf("Revoke(left) error = %v", err)
	}
	waitSignal(t, rightActivation.deactivated, "right source deactivation")
	assertIntegrationRuntimesLocal(t, leftRuntime, rightRuntime, "right-to-left transfer")
}

func TestPeerSessionsRejectCollisionWithoutDisconnectAndAllowRetry(t *testing.T) {
	// TCP buffers simultaneous rejections; net.Pipe has no write buffer and
	// would impose a full-duplex deadlock that the product TCP path does not.
	leftSide, rightSide := openFramedTCP(t)
	leftRuntime := newIntegrationRuntime(t, &integrationInputPort{moves: make(chan [2]int32, 1)})
	rightRuntime := newIntegrationRuntime(t, &integrationInputPort{moves: make(chan [2]int32, 1)})
	leftCoordinator := testWriteCoordinator(t, leftSide)
	rightCoordinator := testWriteCoordinator(t, rightSide)
	leftActivation := newIntegrationSourceActivation()
	rightActivation := newIntegrationSourceActivation()
	leftOutbound, err := NewOutboundController(leftCoordinator, leftRuntime, leftActivation)
	if err != nil {
		t.Fatalf("NewOutboundController(left) error = %v", err)
	}
	rightOutbound, err := NewOutboundController(rightCoordinator, rightRuntime, rightActivation)
	if err != nil {
		t.Fatalf("NewOutboundController(right) error = %v", err)
	}
	leftResponder := &LeaseResponder{
		Coordinator: leftCoordinator,
		Runtime:     leftRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-left", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     &integrationReturnBoundary{},
	}
	rightResponder := &LeaseResponder{
		Coordinator: rightCoordinator,
		Runtime:     rightRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-right", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     &integrationReturnBoundary{},
	}
	leftIdentity := OutboundIdentity{SessionID: "session-left", Epoch: "epoch-left"}
	rightIdentity := OutboundIdentity{SessionID: "session-right", Epoch: "epoch-right"}
	if _, err := leftRuntime.BeginOutbound(context.Background(), leftIdentity.SessionID, leftIdentity.Epoch); err != nil {
		t.Fatalf("BeginOutbound(left) error = %v", err)
	}
	if _, err := rightRuntime.BeginOutbound(context.Background(), rightIdentity.SessionID, rightIdentity.Epoch); err != nil {
		t.Fatalf("BeginOutbound(right) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 2)
	go func() {
		done <- RunSessionWithLifecycle(ctx, leftSide, leftCoordinator, leftRuntime, leftResponder, leftOutbound, time.Now)
	}()
	go func() {
		done <- RunSessionWithLifecycle(ctx, rightSide, rightCoordinator, rightRuntime, rightResponder, rightOutbound, time.Now)
	}()
	t.Cleanup(func() {
		_ = leftSide.Close()
		_ = rightSide.Close()
	})

	requests := []struct {
		writer  FrameWriter
		command transport.ControlCommand
	}{
		{
			writer: leftSide,
			command: transport.ControlCommand{
				Kind: transport.ControlLeaseRequest, SessionID: leftIdentity.SessionID,
				Epoch: leftIdentity.Epoch, Edge: control.ScreenEdgeRight,
			},
		},
		{
			writer: rightSide,
			command: transport.ControlCommand{
				Kind: transport.ControlLeaseRequest, SessionID: rightIdentity.SessionID,
				Epoch: rightIdentity.Epoch, Edge: control.ScreenEdgeLeft,
			},
		},
	}
	writes := make(chan error, len(requests))
	for _, request := range requests {
		request := request
		go func() {
			payload, encodeErr := transport.EncodeControlPayload(request.command)
			if encodeErr != nil {
				writes <- encodeErr
				return
			}
			writes <- request.writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload})
		}()
	}
	for range requests {
		select {
		case <-writes:
		case <-ctx.Done():
			t.Fatalf("simultaneous lease request write did not finish: %v", ctx.Err())
		}
	}
	waitRuntimeState(t, leftRuntime, control.StateLocal, "left collision rejection")
	waitRuntimeState(t, rightRuntime, control.StateLocal, "right collision rejection")
	select {
	case err := <-done:
		t.Fatalf("ordinary collision closed transport: %v", err)
	default:
	}

	assertIntegrationRuntimesLocal(t, leftRuntime, rightRuntime, "simultaneous control requests")
	if leftActivation.Active() || rightActivation.Active() {
		t.Fatalf("source activation survived collision: left=%t right=%t", leftActivation.Active(), rightActivation.Active())
	}
	if err := leftOutbound.Begin(ctx, "retry-session", "retry-epoch", control.ScreenEdgeRight); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, leftActivation.activated, "retry activation on same transport")
	waitRuntimeState(t, rightRuntime, control.StateInboundActive, "retry inbound commit")
	cancel()
	_ = leftSide.Close()
	_ = rightSide.Close()
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("collision session cleanup blocked")
		}
	}
}

func openFramedTCP(t *testing.T) (*framedPipeConnection, *framedPipeConnection) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	left, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = left.Close() })
	right, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = right.Close() })
	wrap := func(c net.Conn) *framedPipeConnection {
		r, err := transport.NewReader(c)
		if err != nil {
			t.Fatal(err)
		}
		w, err := transport.NewWriter(c)
		if err != nil {
			t.Fatal(err)
		}
		return &framedPipeConnection{connection: c, reader: r, writer: w}
	}
	return wrap(left), wrap(right)
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
	sourceCoordinator := testWriteCoordinator(t, sourceSide)
	destinationCoordinator := testWriteCoordinator(t, destinationSide)
	sourceActivation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(sourceCoordinator, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	returnBoundary := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Coordinator: destinationCoordinator,
		Runtime:     destinationRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-new", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     returnBoundary,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	t.Cleanup(func() {
		cancel()
		_ = sourceSide.Close()
		_ = destinationSide.Close()
	})
	go func() {
		done <- RunSessionWithLifecycle(ctx, sourceSide, sourceCoordinator, sourceRuntime, nil, outbound, time.Now)
	}()
	go func() {
		done <- RunSessionWithLifecycle(ctx, destinationSide, destinationCoordinator, destinationRuntime, responder, nil, time.Now)
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
	for range 2 {
		select {
		case sessionErr := <-done:
			if sessionErr == nil {
				t.Fatal("stale-input peer session exited without an error")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("stale-input peer session did not stop")
		}
	}
	if sourceRuntime.Snapshot().Control.State != control.StateLocal || destinationRuntime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("stale input did not fail safe: source=%s destination=%s", sourceRuntime.Snapshot(), destinationRuntime.Snapshot())
	}
	select {
	case move := <-destinationInput.moves:
		t.Fatalf("stale input was injected: %v", move)
	default:
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
	sourceCoordinator := testWriteCoordinator(t, sourceSide)
	destinationCoordinator := testWriteCoordinator(t, destinationSide)
	sourceActivation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(sourceCoordinator, sourceRuntime, sourceActivation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	returns := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Coordinator: destinationCoordinator,
		Runtime:     destinationRuntime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-" + identity.SessionID, accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       integrationCursorEntry{},
		Returns:     returns,
	}
	returnController, err := NewReturnController(destinationCoordinator, destinationRuntime, returns)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() {
		done <- RunSessionWithLifecycle(ctx, sourceSide, sourceCoordinator, sourceRuntime, nil, outbound, time.Now)
	}()
	go func() {
		done <- RunSessionWithLifecycle(ctx, destinationSide, destinationCoordinator, destinationRuntime, responder, nil, time.Now)
	}()
	if err := outbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		identity,
	); err != nil {
		t.Fatalf("BeginBoundary(%s) error = %v", identity.SessionID, err)
	}
	waitSignal(t, sourceActivation.activated, "round-trip source activation")
	if err := SendInputAction(ctx, sourceCoordinator, sourceRuntime, control.InputAction{Kind: control.InputPointerMove, DeltaX: move[0], DeltaY: move[1]}); err != nil {
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

func waitRuntimeState(t *testing.T, runtime *application.SessionRuntime, want control.State, name string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.Snapshot().Control.State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s: %s", name, runtime.Snapshot())
}

func waitMove(t *testing.T, moves <-chan [2]int32, want [2]int32, destination string) {
	t.Helper()
	select {
	case got := <-moves:
		if got != want {
			t.Fatalf("%s move = %v, want %v", destination, got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not inject input", destination)
	}
}

func assertIntegrationRuntimesLocal(
	t *testing.T,
	left *application.SessionRuntime,
	right *application.SessionRuntime,
	phase string,
) {
	t.Helper()
	if left.Snapshot().Control.State != control.StateLocal || right.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("%s did not finish local: left=%s right=%s", phase, left.Snapshot(), right.Snapshot())
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
	activateErr error
	attempted   chan struct{}
	activated   chan struct{}
	deactivated chan struct{}
}

func newIntegrationSourceActivation() *integrationSourceActivation {
	return &integrationSourceActivation{
		attempted:   make(chan struct{}, 1),
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
	select {
	case source.attempted <- struct{}{}:
	default:
	}
	if source.activateErr != nil {
		return source.activateErr
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

type integrationCursorEntry struct {
	committed  chan struct{}
	rolledBack chan struct{}
}

func (integrationCursorEntry) PrepareEntry(context.Context, control.ScreenEdge) error {
	return nil
}
func (entry integrationCursorEntry) CommitEntry() {
	if entry.committed != nil {
		select {
		case entry.committed <- struct{}{}:
		default:
		}
	}
}
func (entry integrationCursorEntry) RollbackEntry(context.Context) (application.CursorRollbackOutcome, error) {
	if entry.rolledBack != nil {
		select {
		case entry.rolledBack <- struct{}{}:
		default:
		}
	}
	return application.CursorRollbackNotRestored, nil
}

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
