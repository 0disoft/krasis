package application

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

type recordingInputPort struct {
	state          *control.PressedState
	pressErr       error
	releaseErr     error
	pressCalls     []control.PressedToken
	releaseCalls   []control.PressedToken
	recordedBefore bool
	moves          [][2]int32
	wheels         [][2]int32
	moveErr        error
	wheelErr       error
}

func (port *recordingInputPort) Press(_ context.Context, token control.PressedToken) error {
	port.pressCalls = append(port.pressCalls, token)
	port.recordedBefore = port.state != nil && port.state.Contains(token)
	return port.pressErr
}

func (port *recordingInputPort) Release(_ context.Context, token control.PressedToken) error {
	port.releaseCalls = append(port.releaseCalls, token)
	return port.releaseErr
}

func (port *recordingInputPort) Move(_ context.Context, deltaX, deltaY int32) error {
	port.moves = append(port.moves, [2]int32{deltaX, deltaY})
	return port.moveErr
}

func (port *recordingInputPort) Wheel(_ context.Context, deltaX, deltaY int32) error {
	port.wheels = append(port.wheels, [2]int32{deltaX, deltaY})
	return port.wheelErr
}

func TestInjectPressRecordsBeforeCallingAdapter(t *testing.T) {
	var state control.PressedState
	port := &recordingInputPort{state: &state}
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}

	if err := InjectPress(context.Background(), &state, port, token); err != nil {
		t.Fatalf("InjectPress returned error: %v", err)
	}
	if !port.recordedBefore || !state.Contains(token) || len(port.pressCalls) != 1 {
		t.Fatalf("press was not journaled first: recorded=%t state=%v calls=%v", port.recordedBefore, state.ReleasePlan(), port.pressCalls)
	}
}

func TestInjectPressRetainsRecoveryStateWhenAdapterFails(t *testing.T) {
	var state control.PressedState
	port := &recordingInputPort{state: &state, pressErr: errEffectFailed}
	token := control.PressedToken{Kind: control.PressedMouseButton, Code: 0}

	err := InjectPress(context.Background(), &state, port, token)
	if !errors.Is(err, errEffectFailed) || !state.Contains(token) {
		t.Fatalf("failed press lost recovery state: error=%v plan=%v", err, state.ReleasePlan())
	}
}

func TestInjectPressForwardsRepeatWithoutDuplicatingState(t *testing.T) {
	var state control.PressedState
	port := &recordingInputPort{state: &state}
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}

	if err := InjectPress(context.Background(), &state, port, token); err != nil {
		t.Fatalf("first InjectPress returned error: %v", err)
	}
	if err := InjectPress(context.Background(), &state, port, token); err != nil {
		t.Fatalf("repeat InjectPress returned error: %v", err)
	}
	if state.Count() != 1 || len(port.pressCalls) != 2 {
		t.Fatalf("repeat press was mishandled: count=%d calls=%v", state.Count(), port.pressCalls)
	}
}

func TestInjectReleaseRemovesOnlyAfterAdapterSuccess(t *testing.T) {
	var state control.PressedState
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 17}
	pressAll(t, &state, token)
	port := &recordingInputPort{state: &state, releaseErr: errReleaseBlocked}

	released, err := InjectRelease(context.Background(), &state, port, token)
	if !errors.Is(err, errReleaseBlocked) || released || !state.Contains(token) {
		t.Fatalf("failed release changed state: released=%t error=%v plan=%v", released, err, state.ReleasePlan())
	}
	port.releaseErr = nil
	released, err = InjectRelease(context.Background(), &state, port, token)
	if err != nil || !released || state.Contains(token) {
		t.Fatalf("successful release did not clear state: released=%t error=%v plan=%v", released, err, state.ReleasePlan())
	}
}

func TestInjectReleaseIgnoresUntrackedToken(t *testing.T) {
	var state control.PressedState
	port := &recordingInputPort{state: &state}
	token := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}

	released, err := InjectRelease(context.Background(), &state, port, token)
	if err != nil || released || len(port.releaseCalls) != 0 {
		t.Fatalf("untracked release reached adapter: released=%t calls=%v error=%v", released, port.releaseCalls, err)
	}
}

func TestInjectReleaseRejectsInvalidUntrackedToken(t *testing.T) {
	var state control.PressedState
	port := &recordingInputPort{state: &state}
	token := control.PressedToken{Kind: "UNKNOWN", Code: 1}

	released, err := InjectRelease(context.Background(), &state, port, token)
	if !errors.Is(err, control.ErrInvalidPressedToken) || released || len(port.releaseCalls) != 0 {
		t.Fatalf("invalid release was not rejected: released=%t calls=%v error=%v", released, port.releaseCalls, err)
	}
}
