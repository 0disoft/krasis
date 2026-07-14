package peer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestBeginOutboundSessionWritesLeaseRequest(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	if err := BeginOutboundSession(context.Background(), testWriteCoordinator(t, writer), runtime, "session-1", "epoch-1", control.ScreenEdgeRight); err != nil {
		t.Fatalf("BeginOutboundSession() error = %v", err)
	}
	command := decodeWrittenControl(t, writer)
	if command.Kind != transport.ControlLeaseRequest || command.SessionID != "session-1" || command.Epoch != "epoch-1" || command.Edge != control.ScreenEdgeRight {
		t.Fatalf("command = %+v", command)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StatePending {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestOutboundActionSinkWritesThroughRuntimeSequence(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	writer := &recordingFrameWriter{}
	sink, err := NewOutboundActionSink(testWriteCoordinator(t, writer), runtime, &recordingCloser{})
	if err != nil {
		t.Fatalf("NewOutboundActionSink() error = %v", err)
	}
	if err := sink.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 2}); err != nil {
		t.Fatalf("SendAction() error = %v", err)
	}
	envelope, action, err := transport.DecodeInputPayload(writer.frames[0].Payload)
	if err != nil {
		t.Fatalf("DecodeInputPayload() error = %v", err)
	}
	if envelope.Sequence != 1 || action.DeltaX != 2 {
		t.Fatalf("payload = envelope:%+v action:%+v", envelope, action)
	}
}

func TestOutboundActionSinkFailCleansRuntime(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	sink, err := NewOutboundActionSink(testWriteCoordinator(t, &recordingFrameWriter{}), runtime, &recordingCloser{})
	if err != nil {
		t.Fatalf("NewOutboundActionSink() error = %v", err)
	}
	if err := sink.Fail(context.Background(), errors.New("capture overflow")); err == nil {
		t.Fatal("Fail() error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestBeginOutboundSessionWriteFailureCleansRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{err: errors.New("write failed")}
	if err := BeginOutboundSession(context.Background(), testWriteCoordinator(t, writer), runtime, "session-1", "epoch-1", control.ScreenEdgeRight); err == nil {
		t.Fatal("BeginOutboundSession() error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasOutboundStream {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestSendInputActionWritesMonotonicEnvelopes(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	writer := &recordingFrameWriter{}
	coordinator := testWriteCoordinator(t, writer)
	for range 2 {
		if err := SendInputAction(context.Background(), coordinator, runtime, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}); err != nil {
			t.Fatalf("SendInputAction() error = %v", err)
		}
	}
	if len(writer.frames) != 2 {
		t.Fatalf("frame count = %d", len(writer.frames))
	}
	for index, frame := range writer.frames {
		envelope, _, err := transport.DecodeInputPayload(frame.Payload)
		if err != nil {
			t.Fatalf("DecodeInputPayload() error = %v", err)
		}
		if envelope.Sequence != uint64(index+1) {
			t.Fatalf("sequence = %d, want %d", envelope.Sequence, index+1)
		}
	}
}

func TestSendInputActionWriteFailureDoesNotReuseSequence(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	writer := &recordingFrameWriter{err: errors.New("uncertain write")}
	if err := SendInputAction(context.Background(), testWriteCoordinator(t, writer), runtime, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}); err == nil {
		t.Fatal("SendInputAction() error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasOutboundStream {
		t.Fatalf("runtime retained failed sequence: %s", snapshot)
	}
}

func TestLeaseResponderGrantsAuthorizedRequest(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	entry := &recordingCursorEntry{runtime: runtime}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, writer),
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       entry,
		Returns:     entry,
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err != nil {
		t.Fatalf("HandleLeaseRequest() error = %v", err)
	}
	grant := decodeWrittenControl(t, writer)
	if grant.Kind != transport.ControlLeaseGrant || grant.LeaseID != "lease-1" {
		t.Fatalf("grant = %+v", grant)
	}
	if len(entry.edges) != 1 || entry.edges[0] != control.ScreenEdgeRight || len(entry.states) != 1 || entry.states[0] != control.StateLocal || len(entry.armed) != 1 || entry.commits != 1 || entry.rollbacks != 0 {
		t.Fatalf("entry = %+v", entry)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive || !snapshot.HasStream {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestLeaseResponderRejectsWithoutActivatingRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	entry := &recordingCursorEntry{}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, writer),
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{},
		Peer:        testAuthenticatedPeer(),
		Entry:       entry,
		Returns:     entry,
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err != nil {
		t.Fatalf("HandleLeaseRequest() error = %v", err)
	}
	if reject := decodeWrittenControl(t, writer); reject.Kind != transport.ControlLeaseReject {
		t.Fatalf("reject = %+v", reject)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s", snapshot)
	}
	if len(entry.edges) != 0 {
		t.Fatalf("rejected request moved cursor: %+v", entry.edges)
	}
}

func TestLeaseResponderEntryFailureDoesNotGrantInbound(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	cause := errors.New("cursor placement failed")
	entry := &recordingCursorEntry{err: cause}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, writer),
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       entry,
		Returns:     entry,
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeLeft}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); !errors.Is(err, cause) {
		t.Fatalf("HandleLeaseRequest() error = %v", err)
	}
	if len(writer.frames) != 0 {
		t.Fatalf("entry failure wrote frames: %+v", writer.frames)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasStream {
		t.Fatalf("entry failure granted inbound: %s", snapshot)
	}
	if len(entry.armed) != 1 || len(entry.disarmed) != 1 {
		t.Fatalf("entry failure did not rollback return detector: armed=%+v disarmed=%+v", entry.armed, entry.disarmed)
	}
	if entry.commits != 0 || entry.rollbacks != 1 {
		t.Fatalf("entry failure compensation = commits:%d rollbacks:%d", entry.commits, entry.rollbacks)
	}
}

func TestLeaseResponderGrantWriteFailureCleansInboundRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	entry := &recordingCursorEntry{}
	writer := &recordingFrameWriter{err: errors.New("write failed")}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, writer),
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       entry,
		Returns:     entry,
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err == nil {
		t.Fatal("HandleLeaseRequest() error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasStream {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
	if len(entry.disarmed) != 1 {
		t.Fatalf("grant write failure did not disarm return detector: %+v", entry.disarmed)
	}
	if entry.commits != 0 || entry.rollbacks != 1 {
		t.Fatalf("grant write failure compensation = commits:%d rollbacks:%d", entry.commits, entry.rollbacks)
	}
}

func TestLeaseResponderGrantRuntimeFailureRollsBackEntry(t *testing.T) {
	runtime, err := application.NewSessionRuntime(failingInboundExecutor{}, &inputPort{})
	if err != nil {
		t.Fatal(err)
	}
	entry := &recordingCursorEntry{}
	writer := &recordingFrameWriter{}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, writer),
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       entry,
		Returns:     entry,
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err == nil {
		t.Fatal("HandleLeaseRequest() error = nil")
	}
	if len(writer.frames) != 0 || entry.commits != 0 || entry.rollbacks != 1 || len(entry.disarmed) != 1 {
		t.Fatalf("runtime failure compensation = frames:%d commits:%d rollbacks:%d disarmed:%d", len(writer.frames), entry.commits, entry.rollbacks, len(entry.disarmed))
	}
}

func TestLeaseResponderJoinsCursorRollbackFailure(t *testing.T) {
	runtime := newLocalRuntime(t)
	writeErr := errors.New("write failed")
	rollbackErr := errors.New("cursor rollback failed")
	entry := &recordingCursorEntry{rollbackErr: rollbackErr}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, &recordingFrameWriter{err: writeErr}),
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:        testAuthenticatedPeer(),
		Entry:       entry,
		Returns:     entry,
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0))
	if !errors.Is(err, writeErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("HandleLeaseRequest() error = %v", err)
	}
}

func newLocalRuntime(t *testing.T) *application.SessionRuntime {
	t.Helper()
	runtime, err := application.NewSessionRuntime(effectExecutor{}, &inputPort{})
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	return runtime
}

func testWriteCoordinator(t *testing.T, writer FrameWriter) *SessionWriteCoordinator {
	t.Helper()
	coordinator, err := NewSessionWriteCoordinator(writer)
	if err != nil {
		t.Fatalf("NewSessionWriteCoordinator() error = %v", err)
	}
	return coordinator
}

func decodeWrittenControl(t *testing.T, writer *recordingFrameWriter) transport.ControlCommand {
	t.Helper()
	if len(writer.frames) != 1 || writer.frames[0].Kind != transport.MessageControl {
		t.Fatalf("frames = %+v", writer.frames)
	}
	command, err := transport.DecodeControlPayload(writer.frames[0].Payload)
	if err != nil {
		t.Fatalf("DecodeControlPayload() error = %v", err)
	}
	return command
}

type recordingFrameWriter struct {
	frames []transport.Frame
	err    error
}

type recordingCloser struct {
	calls int
	err   error
}

func (closer *recordingCloser) Close() error {
	closer.calls++
	return closer.err
}

func (writer *recordingFrameWriter) WriteFrameContext(_ context.Context, frame transport.Frame) error {
	writer.frames = append(writer.frames, frame)
	return writer.err
}

type staticLeaseAuthorizer struct {
	leaseID  string
	accepted bool
	err      error
}

type failingInboundExecutor struct{}

func (failingInboundExecutor) Execute(_ context.Context, effect control.Effect) error {
	if effect == control.EffectBeginInbound {
		return errors.New("begin inbound failed")
	}
	return nil
}

type recordingCursorEntry struct {
	runtime     *application.SessionRuntime
	edges       []control.ScreenEdge
	states      []control.State
	armed       []control.ReturnIntent
	rearmed     []control.ReturnIntent
	disarmed    []control.ReturnIntent
	err         error
	rollbackErr error
	commits     int
	rollbacks   int
}

func (entry *recordingCursorEntry) PrepareEntry(_ context.Context, edge control.ScreenEdge) error {
	entry.edges = append(entry.edges, edge)
	if entry.runtime != nil {
		entry.states = append(entry.states, entry.runtime.Snapshot().Control.State)
	}
	return entry.err
}

func (entry *recordingCursorEntry) CommitEntry() {
	entry.commits++
}

func (entry *recordingCursorEntry) RollbackEntry(context.Context) error {
	entry.rollbacks++
	return entry.rollbackErr
}

func (entry *recordingCursorEntry) ArmReturn(intent control.ReturnIntent, _ control.ScreenEdge) error {
	entry.armed = append(entry.armed, intent)
	return nil
}

func (entry *recordingCursorEntry) RearmReturn(intent control.ReturnIntent) {
	entry.rearmed = append(entry.rearmed, intent)
}

func (entry *recordingCursorEntry) DisarmReturn(intent control.ReturnIntent) {
	entry.disarmed = append(entry.disarmed, intent)
}

func (authorizer staticLeaseAuthorizer) AuthorizeLease(
	context.Context,
	transport.AuthenticatedPeerContext,
	string,
	string,
	time.Time,
) (string, bool, error) {
	return authorizer.leaseID, authorizer.accepted, authorizer.err
}

func testAuthenticatedPeer() transport.AuthenticatedPeerContext {
	return transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("a", 64),
		Role:              transport.AuthenticatedPeerConnector,
	}
}
