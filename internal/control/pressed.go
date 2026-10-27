package control

import (
	"errors"
	"fmt"
)

type PressedKind string

const (
	PressedKeyboardKey PressedKind = "KEYBOARD_KEY"
	PressedMouseButton PressedKind = "MOUSE_BUTTON"
)

var ErrInvalidPressedToken = errors.New("invalid pressed input token")

type PressedToken struct {
	Kind PressedKind
	Code uint32
}

type PressedState struct {
	pressed     map[PressedToken]*pressedEntry
	first, last *pressedEntry
}

type pressedEntry struct {
	token          PressedToken
	previous, next *pressedEntry
}

func (state *PressedState) RecordPress(token PressedToken) (bool, error) {
	if err := token.Validate(); err != nil {
		return false, err
	}
	if state.pressed == nil {
		state.pressed = make(map[PressedToken]*pressedEntry)
	}
	if _, exists := state.pressed[token]; exists {
		return false, nil
	}
	entry := &pressedEntry{token: token, previous: state.last}
	if state.last == nil {
		state.first = entry
	} else {
		state.last.next = entry
	}
	state.last = entry
	state.pressed[token] = entry
	return true, nil
}

func (state *PressedState) RecordRelease(token PressedToken) (bool, error) {
	if err := token.Validate(); err != nil {
		return false, err
	}
	entry, exists := state.pressed[token]
	if !exists {
		return false, nil
	}
	delete(state.pressed, token)
	// Unlink directly, including when newer entries remain after failed releases.
	if entry.previous == nil {
		state.first = entry.next
	} else {
		entry.previous.next = entry.next
	}
	if entry.next == nil {
		state.last = entry.previous
	} else {
		entry.next.previous = entry.previous
	}
	return true, nil
}

func (state *PressedState) Contains(token PressedToken) bool {
	_, exists := state.pressed[token]
	return exists
}

func (state *PressedState) Count() int {
	return len(state.pressed)
}

func (state *PressedState) ReleasePlan() []PressedToken {
	plan := make([]PressedToken, 0, len(state.pressed))
	for entry := state.last; entry != nil; entry = entry.previous {
		plan = append(plan, entry.token)
	}
	return plan
}

func (token PressedToken) Validate() error {
	switch token.Kind {
	case PressedKeyboardKey:
		const allowedKeyboardBits uint32 = 0x1FFFF
		if token.Code&0xFFFF == 0 || token.Code&^allowedKeyboardBits != 0 {
			return fmt.Errorf("%w: invalid encoded keyboard scan code", ErrInvalidPressedToken)
		}
		return nil
	case PressedMouseButton:
		if token.Code < 1 || token.Code > 5 {
			return fmt.Errorf("%w: unsupported mouse button code", ErrInvalidPressedToken)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidPressedToken, token.Kind)
	}
}
