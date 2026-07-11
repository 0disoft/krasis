package application

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestSafetyEffectExecutorMapsReleaseAllToPressedState(t *testing.T) {
	var pressed control.PressedState
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 16}
	pressAll(t, &pressed, token)
	delegate := &recordingEffectExecutor{}
	release := &recordingReleasePort{}
	executor, err := NewSafetyEffectExecutor(delegate, &pressed, release)
	if err != nil {
		t.Fatalf("NewSafetyEffectExecutor returned error: %v", err)
	}

	if err := executor.Execute(context.Background(), control.EffectReleaseAll); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if pressed.Count() != 0 || len(release.calls) != 1 || len(delegate.calls) != 0 {
		t.Fatalf("unexpected release routing: pressed=%d release=%v delegate=%v", pressed.Count(), release.calls, delegate.calls)
	}
}

func TestSafetyEffectExecutorPreservesFailedReleaseForFaultRetry(t *testing.T) {
	var pressed control.PressedState
	token := control.PressedToken{Kind: control.PressedMouseButton, Code: 0}
	pressAll(t, &pressed, token)
	release := &recordingReleasePort{fail: map[control.PressedToken]error{token: errReleaseBlocked}}
	executor, err := NewSafetyEffectExecutor(&recordingEffectExecutor{}, &pressed, release)
	if err != nil {
		t.Fatalf("NewSafetyEffectExecutor returned error: %v", err)
	}

	err = executor.Execute(context.Background(), control.EffectReleaseAll)
	if !errors.Is(err, errReleaseBlocked) || !pressed.Contains(token) {
		t.Fatalf("failed release was lost: error=%v plan=%v", err, pressed.ReleasePlan())
	}
}

func TestSafetyEffectExecutorDelegatesNonReleaseEffects(t *testing.T) {
	var pressed control.PressedState
	delegate := &recordingEffectExecutor{}
	executor, err := NewSafetyEffectExecutor(delegate, &pressed, &recordingReleasePort{})
	if err != nil {
		t.Fatalf("NewSafetyEffectExecutor returned error: %v", err)
	}

	if err := executor.Execute(context.Background(), control.EffectStopRemoteControl); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(delegate.calls) != 1 || delegate.calls[0] != control.EffectStopRemoteControl {
		t.Fatalf("non-release effect was not delegated: %v", delegate.calls)
	}
}

func TestFaultFlowReleasesPressedStateAndConvergesLocal(t *testing.T) {
	var pressed control.PressedState
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	button := control.PressedToken{Kind: control.PressedMouseButton, Code: 0}
	pressAll(t, &pressed, key, button)
	delegate := &recordingEffectExecutor{}
	release := &recordingReleasePort{}
	executor, err := NewSafetyEffectExecutor(delegate, &pressed, release)
	if err != nil {
		t.Fatalf("NewSafetyEffectExecutor returned error: %v", err)
	}
	current := control.Snapshot{State: control.StateInboundActive, SessionID: "session-1", LeaseID: "lease-1"}

	outcome, err := HandleControlEvent(context.Background(), current, control.Event{Kind: control.EventFaultDetected}, executor)
	if err != nil {
		t.Fatalf("HandleControlEvent returned error: %v", err)
	}
	if outcome.Snapshot.State != control.StateLocal || pressed.Count() != 0 || len(release.calls) != 2 {
		t.Fatalf("fault flow did not converge local: outcome=%+v pressed=%v releases=%v", outcome, pressed.ReleasePlan(), release.calls)
	}
}

func TestFaultFlowRetainsFailedReleaseUntilRetry(t *testing.T) {
	var pressed control.PressedState
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 17}
	pressAll(t, &pressed, token)
	delegate := &recordingEffectExecutor{}
	release := &recordingReleasePort{fail: map[control.PressedToken]error{token: errReleaseBlocked}}
	executor, err := NewSafetyEffectExecutor(delegate, &pressed, release)
	if err != nil {
		t.Fatalf("NewSafetyEffectExecutor returned error: %v", err)
	}
	current := control.Snapshot{State: control.StateOutboundActive, SessionID: "session-1", LeaseID: "lease-1"}

	failed, err := HandleControlEvent(context.Background(), current, control.Event{Kind: control.EventFaultDetected}, executor)
	if !errors.Is(err, errReleaseBlocked) || failed.Snapshot.State != control.StateFault || !pressed.Contains(token) {
		t.Fatalf("failed release was not retained: outcome=%+v plan=%v error=%v", failed, pressed.ReleasePlan(), err)
	}

	release.fail = nil
	recovered, err := HandleControlEvent(context.Background(), failed.Snapshot, control.Event{Kind: control.EventFaultDetected}, executor)
	if err != nil || recovered.Snapshot.State != control.StateLocal || pressed.Count() != 0 {
		t.Fatalf("release retry did not recover: outcome=%+v plan=%v error=%v", recovered, pressed.ReleasePlan(), err)
	}
}
