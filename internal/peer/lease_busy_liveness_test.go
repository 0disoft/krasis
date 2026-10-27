package peer

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestBusyLeaseRequestDoesNotBlockHeartbeatBehindNativeInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		port := &uncooperativeHeartbeatInput{entered: make(chan struct{}), resume: make(chan struct{})}
		runtime, err := application.NewSessionRuntime(effectExecutor{}, port)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.GrantInbound(ctx, control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now()); err != nil {
			t.Fatal(err)
		}
		inputDone := make(chan error, 1)
		go func() {
			inputDone <- runtime.HandleInputAction(ctx, control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
		}()
		<-port.entered // The native operation owns runtime.mu until resumed.
		writer := &recordingFrameWriter{}
		coordinator := testWriteCoordinator(t, writer)
		entry := &recordingCursorEntry{}
		responder := &LeaseResponder{Coordinator: coordinator, Runtime: runtime,
			Authorizer: staticLeaseAuthorizer{err: errors.New("busy request must not authorize")},
			Peer:       testAuthenticatedPeer(), Entry: entry, Returns: entry}
		requestDone, requestExited := make(chan error, 1), make(chan struct{})
		go func() {
			defer close(requestExited)
			requestDone <- responder.HandleLeaseRequest(ctx, transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "other", Epoch: "other-e", Edge: control.ScreenEdgeRight}, time.Now())
		}()
		var pulseExited chan struct{}
		defer func() {
			close(port.resume)
			<-inputDone
			<-requestExited
			if pulseExited != nil {
				<-pulseExited
			}
		}()
		synctest.Wait()
		select {
		case err := <-requestDone:
			if err != nil {
				t.Error("busy request was not rejected normally", err)
			} else if command := decodeWrittenControl(t, writer); command.Kind != transport.ControlLeaseReject || command.SessionID != "other" || command.Epoch != "other-e" {
				t.Error("busy rejection lost the requested identity", command)
			}
		default:
			t.Error("busy rejection waited for the native input owner")
		}
		pulseDone := make(chan error, 1)
		pulseExited = make(chan struct{})
		go func() {
			defer close(pulseExited)
			pulse, err := PulseLiveness(ctx, coordinator, runtime, time.Now, 3*time.Second)
			if err == nil && (!pulse.Sent || pulse.SessionID != "s") {
				err = errors.New("active lease heartbeat was not sent")
			}
			pulseDone <- err
		}()
		synctest.Wait()
		select {
		case err := <-pulseDone:
			if err != nil {
				t.Error(err)
			}
		default:
			t.Error("busy rejection retained the heartbeat writer permit")
		}
		if len(entry.edges) != 0 || runtime.LeaseWatch().SessionID != "s" {
			t.Error("busy request changed cursor or lease ownership")
		}
	})
}
