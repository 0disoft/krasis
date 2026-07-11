package wininput

import (
	"errors"
	"fmt"
	"sync"

	"github.com/0disoft/krasis/internal/control"
)

const ReturnIntentQueueCapacity = 1

type InputAdapter struct {
	returns *returnBoundaryState
}

type returnBoundaryState struct {
	mu         sync.Mutex
	intent     control.ReturnIntent
	sourceEdge control.ScreenEdge
	armed      bool
	pressed    control.PressedState
	intents    chan control.ReturnIntent
}

func newInputAdapter() *InputAdapter {
	return &InputAdapter{returns: &returnBoundaryState{intents: make(chan control.ReturnIntent, ReturnIntentQueueCapacity)}}
}

func (adapter *InputAdapter) ArmReturn(intent control.ReturnIntent, sourceEdge control.ScreenEdge) error {
	state, err := adapter.returnState()
	if err != nil {
		return err
	}
	if err := intent.Validate(); err != nil {
		return err
	}
	if sourceEdge != control.ScreenEdgeLeft && sourceEdge != control.ScreenEdgeRight {
		return fmt.Errorf("%w: return source edge must be LEFT or RIGHT", ErrUnsupportedInput)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for {
		select {
		case <-state.intents:
		default:
			state.intent = intent
			state.sourceEdge = sourceEdge
			state.pressed = control.PressedState{}
			state.armed = true
			return nil
		}
	}
}

func (adapter *InputAdapter) RearmReturn(intent control.ReturnIntent) {
	state, err := adapter.returnState()
	if err != nil {
		return
	}
	state.mu.Lock()
	if state.intent == intent {
		state.armed = true
	}
	state.mu.Unlock()
}

func (adapter *InputAdapter) DisarmReturn(intent control.ReturnIntent) {
	state, err := adapter.returnState()
	if err != nil {
		return
	}
	state.mu.Lock()
	if state.intent == intent {
		state.armed = false
		state.intent = control.ReturnIntent{}
		state.sourceEdge = ""
		state.pressed = control.PressedState{}
	}
	state.mu.Unlock()
}

func (adapter *InputAdapter) ReturnIntents() <-chan control.ReturnIntent {
	if adapter == nil || adapter.returns == nil {
		return nil
	}
	return adapter.returns.intents
}

func (adapter *InputAdapter) recordReturnInput(token control.PressedToken, pressed bool) error {
	state, err := adapter.returnState()
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.armed {
		return nil
	}
	if pressed {
		_, err = state.pressed.RecordPress(token)
	} else {
		_, err = state.pressed.RecordRelease(token)
	}
	return err
}

func (adapter *InputAdapter) observeReturnPosition(pointerX, screenLeft, screenWidth int32) error {
	state, err := adapter.returnState()
	if err != nil {
		return err
	}
	if screenWidth <= 1 {
		return fmt.Errorf("%w: return screen width is invalid", ErrUnsupportedInput)
	}
	right := int64(screenLeft) + int64(screenWidth) - 1
	if right > int64(^uint32(0)>>1) || right < -int64(^uint32(0)>>1)-1 {
		return fmt.Errorf("%w: return screen coordinates overflow int32", ErrUnsupportedInput)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.armed || state.pressed.Count() != 0 {
		return nil
	}
	reached := state.sourceEdge == control.ScreenEdgeRight && pointerX <= screenLeft
	if state.sourceEdge == control.ScreenEdgeLeft {
		reached = pointerX >= int32(right)
	}
	if !reached {
		return nil
	}
	state.armed = false
	select {
	case state.intents <- state.intent:
	default:
	}
	return nil
}

func (adapter *InputAdapter) returnState() (*returnBoundaryState, error) {
	if adapter == nil || adapter.returns == nil || adapter.returns.intents == nil {
		return nil, errors.New("input adapter return boundary is not initialized")
	}
	return adapter.returns, nil
}
