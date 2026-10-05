//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type delegatedInput struct {
	calls    []string
	err      error
	cursor   func() point
	position point
}

func (p *delegatedInput) Press(context.Context, control.PressedToken) error {
	p.calls = append(p.calls, "press")
	return p.err
}
func (p *delegatedInput) Release(context.Context, control.PressedToken) error {
	p.calls = append(p.calls, "release")
	return p.err
}
func (p *delegatedInput) Move(context.Context, int32, int32) error {
	p.calls = append(p.calls, "move")
	return p.err
}
func (p *delegatedInput) Wheel(context.Context, int32, int32) error {
	p.calls = append(p.calls, "wheel")
	return p.err
}

func (p *delegatedInput) CursorPosition(context.Context) (int32, int32, error) {
	position := p.position
	if p.cursor != nil {
		position = p.cursor()
	}
	return position.X, position.Y, p.err
}
func (p *delegatedInput) SetCursorPosition(_ context.Context, x, y int32) error {
	p.calls = append(p.calls, "cursor-set")
	if p.err == nil {
		p.position = point{X: x, Y: y}
	}
	return p.err
}

func TestDelegatedInputPreservesReturnTrackingWithoutLocalInjection(t *testing.T) {
	oldSend, oldCursor := callSendInput, callGetCursorPos
	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
		t.Fatal("delegated input called local SendInput")
		return 0, 0, nil
	}
	x := int32(0)
	callGetCursorPos = func(*point) (uintptr, uintptr, error) {
		t.Fatal("delegated motion queried local cursor")
		return 0, 0, nil
	}
	t.Cleanup(func() { callSendInput = oldSend; callGetCursorPos = oldCursor })
	port := &delegatedInput{cursor: func() point { return point{X: x} }}
	adapter, err := NewInputAdapterWithInjection(10, 1920, port)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	intent := control.ReturnIntent{SessionID: "session", Epoch: "epoch"}
	if err := adapter.ArmReturn(intent, control.ScreenEdgeRight); err != nil {
		t.Fatal(err)
	}
	left, width := systemMetric(smXVIRTUALSCREEN), systemMetric(smCXVIRTUALSCREEN)
	x = left + 20
	if err := adapter.Move(ctx, 20, 0); err != nil {
		t.Fatal(err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if err := adapter.Press(ctx, key); err != nil {
		t.Fatal(err)
	}
	x = left
	if err := adapter.Move(ctx, -20, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-adapter.ReturnIntents():
		t.Fatal("returned while key held")
	default:
	}
	if err := adapter.Release(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Move(ctx, -1, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-adapter.ReturnIntents():
		if got != intent {
			t.Fatal("wrong return lease")
		}
	default:
		t.Fatalf("missing return intent (width %d)", width)
	}
	if err := adapter.Wheel(ctx, 0, 120); err != nil {
		t.Fatal(err)
	}
	port.err = &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}
	if err := adapter.Press(ctx, key); !errors.Is(err, port.err) {
		t.Fatal("lost uncertain input result")
	}
}
