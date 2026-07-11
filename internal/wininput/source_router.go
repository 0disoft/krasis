package wininput

import (
	"errors"

	"github.com/0disoft/krasis/internal/control"
)

type BoundaryObserver interface {
	Observe(control.InputAction, int32) error
}

func routePhysicalAction(
	session *SourceSession,
	boundary BoundaryObserver,
	action control.InputAction,
	pointerX int32,
) bool {
	if session == nil {
		return false
	}
	if boundary != nil {
		if err := boundary.Observe(action, pointerX); err != nil {
			if !errors.Is(err, control.ErrBoundaryIntentQueueFull) && session.Active() {
				session.Abort(err)
			}
			return false
		}
	}
	return session.Offer(action)
}
