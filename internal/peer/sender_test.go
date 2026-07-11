package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestBeginOutboundSessionWritesLeaseRequest(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	if err := BeginOutboundSession(context.Background(), writer, runtime, "session-1", "epoch-1"); err != nil {
		t.Fatalf("BeginOutboundSession() error = %v", err)
	}
	command := decodeWrittenControl(t, writer)
	if command.Kind != transport.ControlLeaseRequest || command.SessionID != "session-1" || command.Epoch != "epoch-1" {
		t.Fatalf("command = %+v", command)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StatePending {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestBeginOutboundSessionWriteFailureCleansRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{err: errors.New("write failed")}
	if err := BeginOutboundSession(context.Background(), writer, runtime, "session-1", "epoch-1"); err == nil {
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
	for range 2 {
		if err := SendInputAction(context.Background(), writer, runtime, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}); err != nil {
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
	if err := SendInputAction(context.Background(), writer, runtime, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}); err == nil {
		t.Fatal("SendInputAction() error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasOutboundStream {
		t.Fatalf("runtime retained failed sequence: %s", snapshot)
	}
}

func TestLeaseResponderGrantsAuthorizedRequest(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	responder := &LeaseResponder{
		Writer:     writer,
		Runtime:    runtime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1"}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err != nil {
		t.Fatalf("HandleLeaseRequest() error = %v", err)
	}
	grant := decodeWrittenControl(t, writer)
	if grant.Kind != transport.ControlLeaseGrant || grant.LeaseID != "lease-1" {
		t.Fatalf("grant = %+v", grant)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive || !snapshot.HasStream {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestLeaseResponderRejectsWithoutActivatingRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	responder := &LeaseResponder{
		Writer:     writer,
		Runtime:    runtime,
		Authorizer: staticLeaseAuthorizer{},
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1"}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err != nil {
		t.Fatalf("HandleLeaseRequest() error = %v", err)
	}
	if reject := decodeWrittenControl(t, writer); reject.Kind != transport.ControlLeaseReject {
		t.Fatalf("reject = %+v", reject)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestLeaseResponderGrantWriteFailureCleansInboundRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	responder := &LeaseResponder{
		Writer:     &recordingFrameWriter{err: errors.New("write failed")},
		Runtime:    runtime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
	}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1"}
	if err := responder.HandleLeaseRequest(context.Background(), command, time.Unix(100, 0)); err == nil {
		t.Fatal("HandleLeaseRequest() error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasStream {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
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

func (writer *recordingFrameWriter) WriteFrameContext(_ context.Context, frame transport.Frame) error {
	writer.frames = append(writer.frames, frame)
	return writer.err
}

type staticLeaseAuthorizer struct {
	leaseID  string
	accepted bool
	err      error
}

func (authorizer staticLeaseAuthorizer) AuthorizeLease(
	context.Context,
	string,
	string,
	time.Time,
) (string, bool, error) {
	return authorizer.leaseID, authorizer.accepted, authorizer.err
}
