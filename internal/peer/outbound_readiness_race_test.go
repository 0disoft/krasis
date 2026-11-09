package peer

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type blockingLeaseAuthorizer struct {
	entered chan struct{}
	release chan struct{}
}

func (authorizer *blockingLeaseAuthorizer) AuthorizeLease(
	context.Context,
	transport.AuthenticatedPeerContext,
	string,
	string,
	time.Time,
) (string, bool, error) {
	close(authorizer.entered)
	<-authorizer.release
	return "lease", true, nil
}

type readinessSignalingSource struct {
	active      bool
	checked     chan struct{}
	deactivated int
}

func (source *readinessSignalingSource) Active() bool {
	select {
	case source.checked <- struct{}{}:
	default:
	}
	return source.active
}

func (source *readinessSignalingSource) Activate() error {
	source.active = true
	return nil
}

func (source *readinessSignalingSource) Deactivate() {
	source.active = false
	source.deactivated++
}

func TestOutboundBeginRechecksReadinessAfterInboundPreparationQueued(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime := newLocalRuntime(t)
		writer := &recordingFrameWriter{}
		coordinator := testWriteCoordinator(t, writer)
		entry := &recordingCursorEntry{}
		authorizer := &blockingLeaseAuthorizer{entered: make(chan struct{}), release: make(chan struct{})}
		responder := &LeaseResponder{
			Coordinator: coordinator,
			Runtime:     runtime,
			Authorizer:  authorizer,
			Peer:        testAuthenticatedPeer(),
			Entry:       entry,
			Returns:     entry,
		}
		source := &readinessSignalingSource{checked: make(chan struct{}, 1)}
		controller, err := NewOutboundController(coordinator, runtime, source)
		if err != nil {
			t.Fatal(err)
		}

		requestDone := make(chan error, 1)
		go func() {
			requestDone <- responder.HandleLeaseRequest(context.Background(), transport.ControlCommand{
				Kind: transport.ControlLeaseRequest, SessionID: "inbound", Epoch: "inbound-epoch", Edge: control.ScreenEdgeLeft,
			}, time.Unix(100, 0))
		}()
		<-authorizer.entered

		beginDone := make(chan error, 1)
		go func() {
			beginDone <- controller.Begin(context.Background(), "outbound", "outbound-epoch", control.ScreenEdgeRight)
		}()
		<-source.checked
		// Both handlers are now blocked: the responder before inbound preparation,
		// and Begin after its initial Ready check while waiting for the coordinator.
		synctest.Wait()
		close(authorizer.release)

		if err := <-requestDone; err != nil {
			t.Fatalf("inbound preparation failed: %v", err)
		}
		if err := <-beginDone; !errors.Is(err, ErrOutboundNotReady) {
			t.Fatalf("Begin() error = %v, want ErrOutboundNotReady", err)
		}
		if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundPending {
			t.Fatalf("contended Begin changed inbound preparation: %s", snapshot.Control.State)
		}
		if source.active || source.deactivated != 0 {
			t.Fatalf("contended Begin changed source ownership: active=%t deactivated=%d", source.active, source.deactivated)
		}
		if len(writer.frames) != 1 {
			t.Fatalf("frames = %d, want only the inbound grant", len(writer.frames))
		}
		if grant := decodeWrittenControl(t, writer); grant.Kind != transport.ControlLeaseGrant || grant.SessionID != "inbound" {
			t.Fatalf("inbound grant = %+v", grant)
		}
	})
}
