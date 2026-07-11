package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestOutboundControllerActivatesSourceOnlyAfterGrant(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	source := &recordingSourceActivation{}
	controller, err := NewOutboundController(writer, runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	if err := controller.Begin(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if source.active {
		t.Fatal("source activated before grant")
	}
	grant := transport.ControlCommand{
		Kind:      transport.ControlLeaseGrant,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	}
	if err := controller.GrantPeer(context.Background(), grant, time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantPeer() error = %v", err)
	}
	if !source.active || source.activations != 1 {
		t.Fatalf("source = %+v", source)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateOutboundActive {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestOutboundControllerActivationFailureCleansGrantedLease(t *testing.T) {
	runtime := newLocalRuntime(t)
	source := &recordingSourceActivation{activateErr: errors.New("hook worker unavailable")}
	controller, err := NewOutboundController(&recordingFrameWriter{}, runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	if err := controller.Begin(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	grant := transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"}
	if err := controller.GrantPeer(context.Background(), grant, time.Unix(100, 0)); err == nil {
		t.Fatal("GrantPeer() error = nil")
	}
	if source.active {
		t.Fatal("source remained active after activation failure")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestLifecycleDispatcherUsesOutboundController(t *testing.T) {
	runtime := newLocalRuntime(t)
	source := &recordingSourceActivation{}
	controller, err := NewOutboundController(&recordingFrameWriter{}, runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	if err := controller.Begin(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	frame := controlFrame(t, transport.ControlCommand{
		Kind:      transport.ControlLeaseGrant,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	})
	if err := DispatchControlFrameWithLifecycle(
		context.Background(),
		frame,
		time.Unix(100, 0),
		runtime,
		nil,
		controller,
	); err != nil {
		t.Fatalf("DispatchControlFrameWithLifecycle() error = %v", err)
	}
	if !source.active {
		t.Fatal("lifecycle dispatcher did not activate source")
	}
}

func TestOutboundControllerRevokeDisablesSourceBeforeRuntimeCleanup(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	source := &recordingSourceActivation{active: true}
	controller, err := NewOutboundController(&recordingFrameWriter{}, runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	revoke := transport.ControlCommand{Kind: transport.ControlLeaseRevoke, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"}
	if err := controller.RevokePeer(context.Background(), revoke); err != nil {
		t.Fatalf("RevokePeer() error = %v", err)
	}
	if source.active {
		t.Fatal("source remained active after revoke")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

type recordingSourceActivation struct {
	active      bool
	activations int
	activateErr error
}

func (source *recordingSourceActivation) Activate() error {
	if source.activateErr != nil {
		return source.activateErr
	}
	source.active = true
	source.activations++
	return nil
}

func (source *recordingSourceActivation) Deactivate()  { source.active = false }
func (source *recordingSourceActivation) Active() bool { return source.active }
