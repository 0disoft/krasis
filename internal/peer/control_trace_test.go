package peer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime/trace"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

type traceReadConnection struct{ sent bool }

func (c *traceReadConnection) ReadFrame() (transport.Frame, error) {
	if c.sent {
		return transport.Frame{}, io.EOF
	}
	c.sent = true
	payload, err := transport.EncodeControlPayload(transport.ControlCommand{Kind: transport.ControlPrepareInput})
	return transport.Frame{Kind: transport.MessageControl, Payload: payload}, err
}
func (*traceReadConnection) Close() error { return nil }

func TestPeerTraceRegionsPreserveResultsAndUseFixedLabels(t *testing.T) {
	var output bytes.Buffer
	if err := trace.Start(&output); err != nil {
		t.Fatal(err)
	}
	tracing := true
	defer func() {
		if tracing {
			trace.Stop()
		}
	}()
	ctx := context.Background()
	reader := startInputReadAhead(ctx, &traceReadConnection{})
	if frame, err := reader.ReadFrame(); err != nil || frame.Kind != transport.MessageControl {
		t.Fatal("trace changed frame read", frame.Kind, err)
	}
	if _, err := reader.ReadFrame(); err != io.EOF {
		t.Fatal("trace changed read error", err)
	}
	<-reader.done
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	cause := errors.New("operation failed")
	if err := coordinator.Execute(ctx, func(FrameWriter) error { return cause }); err != cause {
		t.Fatal("trace changed original operation error", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := coordinator.Execute(canceled, func(FrameWriter) error {
		t.Error("canceled operation ran")
		return nil
	}); err != context.Canceled {
		t.Fatal("trace changed cancellation", err)
	}
	payload, err := transport.EncodeControlPayload(transport.ControlCommand{Kind: transport.ControlPrepareInput})
	if err != nil {
		t.Fatal(err)
	}
	if err := DispatchControlFrame(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload}, time.Now(), newLocalRuntime(t), nil); err != nil {
		t.Fatal(err)
	}
	traceControlRead(ctx, []byte("private-payload-not-a-trace-label"))
	trace.Stop()
	tracing = false
	for _, label := range []string{"krasis.peer.wire_read", "krasis.peer.read_enqueue", "krasis.peer.coordinator_wait", "krasis.peer.coordinator_operation", "krasis.peer.prepare_input", "krasis.peer.control_read", "krasis.peer.control_unknown"} {
		if !bytes.Contains(output.Bytes(), []byte(label)) {
			t.Error("missing trace label", label)
		}
	}
	if bytes.Contains(output.Bytes(), []byte("private-payload-not-a-trace-label")) {
		t.Fatal("wire payload leaked into trace")
	}
	if got := controlTraceName(transport.ControlCommandKind(255)); got != "krasis.peer.control_unknown" {
		t.Fatal("trace leaked an untrusted label", got)
	}
}

func TestUntracedCoordinatorRegionsDoNotAllocate(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("allocation assertion requires tracing disabled")
	}
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	ctx := context.Background()
	operation := func(FrameWriter) error { return nil }
	if allocations := testing.AllocsPerRun(1000, func() {
		if err := coordinator.Execute(ctx, operation); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatal("disabled trace adds coordinator allocations", allocations)
	}
}
