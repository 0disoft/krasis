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
	pressed map[PressedToken]struct{}
	order   []PressedToken
}

func (state *PressedState) RecordPress(token PressedToken) (bool, error) {
	if err := validatePressedToken(token); err != nil {
		return false, err
	}
	if state.pressed == nil {
		state.pressed = make(map[PressedToken]struct{})
	}
	if _, exists := state.pressed[token]; exists {
		return false, nil
	}
	state.pressed[token] = struct{}{}
	state.order = append(state.order, token)
	return true, nil
}

func (state *PressedState) RecordRelease(token PressedToken) (bool, error) {
	if err := validatePressedToken(token); err != nil {
		return false, err
	}
	if _, exists := state.pressed[token]; !exists {
		return false, nil
	}
	delete(state.pressed, token)
	for index, candidate := range state.order {
		if candidate == token {
			state.order = append(state.order[:index], state.order[index+1:]...)
			break
		}
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
	plan := make([]PressedToken, 0, len(state.order))
	for index := len(state.order) - 1; index >= 0; index-- {
		plan = append(plan, state.order[index])
	}
	return plan
}

func validatePressedToken(token PressedToken) error {
	switch token.Kind {
	case PressedKeyboardKey, PressedMouseButton:
		return nil
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidPressedToken, token.Kind)
	}
}
