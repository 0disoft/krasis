package wininput

import (
	"errors"
	"fmt"
	"sync"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

const ReturnIntentQueueCapacity = 1

const defaultReturnRearmDistance int32 = 32

type InputAdapter struct {
	returns   *returnBoundaryState
	entry     *cursorEntryState
	injection application.InputPort
}

// NewInputAdapterWithInjection keeps cursor entry and return tracking local.
// The immutable delegate owns injection; errors never fall back to SendInput.
func NewInputAdapterWithInjection(distance, screenWidth int32, port application.InputPort) (*InputAdapter, error) {
	if port == nil {
		return nil, ErrUnsupportedInput
	}
	adapter, err := NewInputAdapterWithReturnRearmDistance(distance, screenWidth)
	if err != nil {
		return nil, err
	}
	adapter.injection = port
	return adapter, nil
}

type cursorEntryState struct {
	mu           sync.Mutex
	pending      bool
	enteredKnown bool
	previousX    int32
	previousY    int32
	enteredX     int32
	enteredY     int32
}

type returnBoundaryState struct {
	mu            sync.Mutex
	intent        control.ReturnIntent
	sourceEdge    control.ScreenEdge
	armed         bool
	returnReady   bool
	rearmDistance int32
	pressed       control.PressedState
	intents       chan control.ReturnIntent
}

func newInputAdapter() *InputAdapter {
	adapter, err := newInputAdapterWithReturnRearmDistance(defaultReturnRearmDistance)
	if err != nil {
		panic(err)
	}
	return adapter
}

func NewInputAdapterWithReturnRearmDistance(distance, screenWidth int32) (*InputAdapter, error) {
	if screenWidth <= 1 {
		return nil, fmt.Errorf("%w: return screen width is invalid", ErrUnsupportedInput)
	}
	if distance >= screenWidth {
		return nil, fmt.Errorf("%w: return rearm distance must be smaller than the screen width", ErrUnsupportedInput)
	}
	return newInputAdapterWithReturnRearmDistance(distance)
}

func newInputAdapterWithReturnRearmDistance(distance int32) (*InputAdapter, error) {
	if distance <= cursorEntryInset {
		return nil, fmt.Errorf("%w: return rearm distance must be greater than cursor entry inset", ErrUnsupportedInput)
	}
	return &InputAdapter{
		returns: &returnBoundaryState{
			rearmDistance: distance,
			intents:       make(chan control.ReturnIntent, ReturnIntentQueueCapacity),
		},
		entry: &cursorEntryState{},
	}, nil
}

func (adapter *InputAdapter) CommitEntry() {
	state, err := adapter.cursorEntryState()
	if err != nil {
		return
	}
	state.mu.Lock()
	state.clear()
	state.mu.Unlock()
}

func (adapter *InputAdapter) cursorEntryState() (*cursorEntryState, error) {
	if adapter == nil || adapter.entry == nil {
		return nil, errors.New("input adapter cursor entry is not initialized")
	}
	return adapter.entry, nil
}

func (state *cursorEntryState) clear() {
	state.pending = false
	state.enteredKnown = false
	state.previousX = 0
	state.previousY = 0
	state.enteredX = 0
	state.enteredY = 0
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
			state.returnReady = false
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
		state.returnReady = false
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
	if !state.returnReady {
		state.returnReady = returnBoundaryDepartedEntryEdge(
			state.sourceEdge,
			pointerX,
			screenLeft,
			int32(right),
			state.rearmDistance,
		)
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

func returnBoundaryDepartedEntryEdge(
	sourceEdge control.ScreenEdge,
	pointerX int32,
	left int32,
	right int32,
	distance int32,
) bool {
	switch sourceEdge {
	case control.ScreenEdgeRight:
		return int64(pointerX) >= int64(left)+int64(distance)
	case control.ScreenEdgeLeft:
		return int64(pointerX) <= int64(right)-int64(distance)
	default:
		return false
	}
}

func (adapter *InputAdapter) returnState() (*returnBoundaryState, error) {
	if adapter == nil || adapter.returns == nil || adapter.returns.intents == nil {
		return nil, errors.New("input adapter return boundary is not initialized")
	}
	return adapter.returns, nil
}
