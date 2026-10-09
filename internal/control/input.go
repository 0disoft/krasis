package control

import (
	"errors"
	"fmt"
)

type InputActionKind string

// Relative frames share the protected worker's signed delta contract.
const MaxInputDelta = 32767

const (
	InputKeyDown     InputActionKind = "KEY_DOWN"
	InputKeyUp       InputActionKind = "KEY_UP"
	InputButtonDown  InputActionKind = "BUTTON_DOWN"
	InputButtonUp    InputActionKind = "BUTTON_UP"
	InputPointerMove InputActionKind = "POINTER_MOVE"
	InputWheel       InputActionKind = "WHEEL"
)

var ErrInvalidInputAction = errors.New("invalid normalized input action")

type InputAction struct {
	Kind   InputActionKind
	Code   uint32
	DeltaX int32
	DeltaY int32
}

func (action InputAction) Validate() error {
	switch action.Kind {
	case InputKeyDown, InputKeyUp, InputButtonDown, InputButtonUp:
		if action.DeltaX != 0 || action.DeltaY != 0 {
			return fmt.Errorf("%w: discrete input must not include deltas", ErrInvalidInputAction)
		}
		token, _, _ := action.PressedToken()
		if err := token.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidInputAction, err)
		}
	case InputPointerMove, InputWheel:
		if action.Code != 0 {
			return fmt.Errorf("%w: relative input must not include a code", ErrInvalidInputAction)
		}
		if action.DeltaX == 0 && action.DeltaY == 0 {
			return fmt.Errorf("%w: relative input requires a non-zero delta", ErrInvalidInputAction)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidInputAction, action.Kind)
	}
	return nil
}

func (action InputAction) PressedToken() (PressedToken, bool, bool) {
	switch action.Kind {
	case InputKeyDown:
		return PressedToken{Kind: PressedKeyboardKey, Code: action.Code}, true, true
	case InputKeyUp:
		return PressedToken{Kind: PressedKeyboardKey, Code: action.Code}, false, true
	case InputButtonDown:
		return PressedToken{Kind: PressedMouseButton, Code: action.Code}, true, true
	case InputButtonUp:
		return PressedToken{Kind: PressedMouseButton, Code: action.Code}, false, true
	default:
		return PressedToken{}, false, false
	}
}
