package control

import (
	"errors"
	"testing"
)

func TestInputActionValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  InputAction
		wantErr bool
	}{
		{name: "key", action: InputAction{Kind: InputKeyDown, Code: 65}},
		{name: "button", action: InputAction{Kind: InputButtonUp, Code: 1}},
		{name: "move", action: InputAction{Kind: InputPointerMove, DeltaX: -4, DeltaY: 2}},
		{name: "wheel", action: InputAction{Kind: InputWheel, DeltaY: 120}},
		{name: "unknown", action: InputAction{Kind: "UNKNOWN"}, wantErr: true},
		{name: "discrete delta", action: InputAction{Kind: InputKeyDown, Code: 65, DeltaX: 1}, wantErr: true},
		{name: "zero key code", action: InputAction{Kind: InputKeyDown}, wantErr: true},
		{name: "unsupported key bits", action: InputAction{Kind: InputKeyUp, Code: 1 << 31}, wantErr: true},
		{name: "zero button code", action: InputAction{Kind: InputButtonDown}, wantErr: true},
		{name: "unsupported button code", action: InputAction{Kind: InputButtonUp, Code: 6}, wantErr: true},
		{name: "relative code", action: InputAction{Kind: InputPointerMove, Code: 1, DeltaX: 1}, wantErr: true},
		{name: "empty relative", action: InputAction{Kind: InputWheel}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.action.Validate()
			if test.wantErr && !errors.Is(err, ErrInvalidInputAction) {
				t.Fatalf("Validate() error = %v, want ErrInvalidInputAction", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestInputActionPressedToken(t *testing.T) {
	tests := []struct {
		action  InputAction
		kind    PressedKind
		pressed bool
		ok      bool
	}{
		{action: InputAction{Kind: InputKeyDown, Code: 65}, kind: PressedKeyboardKey, pressed: true, ok: true},
		{action: InputAction{Kind: InputKeyUp, Code: 65}, kind: PressedKeyboardKey, pressed: false, ok: true},
		{action: InputAction{Kind: InputButtonDown, Code: 1}, kind: PressedMouseButton, pressed: true, ok: true},
		{action: InputAction{Kind: InputButtonUp, Code: 1}, kind: PressedMouseButton, pressed: false, ok: true},
		{action: InputAction{Kind: InputPointerMove, DeltaX: 1}},
	}
	for _, test := range tests {
		token, pressed, ok := test.action.PressedToken()
		if ok != test.ok || pressed != test.pressed || (ok && (token.Kind != test.kind || token.Code != test.action.Code)) {
			t.Fatalf("PressedToken(%s) = (%v, %t, %t)", test.action.Kind, token, pressed, ok)
		}
	}
}
