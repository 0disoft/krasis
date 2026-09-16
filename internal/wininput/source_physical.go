package wininput

import "github.com/0disoft/krasis/internal/control"

type physicalPress struct {
	button bool
	code   uint32
}

// Caller owns physicalMu. Only the physical hook path populates this ledger;
// injected events never enter it. Key repeat is idempotent.
func (session *SourceSession) observePhysicalPress(action control.InputAction) {
	if action.Validate() != nil {
		return
	}
	var down bool
	key := physicalPress{code: action.Code}
	switch action.Kind {
	case control.InputKeyDown:
		down = true
	case control.InputKeyUp:
	case control.InputButtonDown:
		key.button, down = true, true
	case control.InputButtonUp:
		key.button = true
	default:
		return
	}
	if down {
		if session.physicalHeld == nil {
			session.physicalHeld = make(map[physicalPress]struct{})
		}
		session.physicalHeld[key] = struct{}{}
	} else {
		delete(session.physicalHeld, key)
	}
}
