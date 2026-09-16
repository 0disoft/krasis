package wininput

import (
	"errors"

	"github.com/0disoft/krasis/internal/control"
)

type BoundaryObserver interface {
	Observe(control.InputAction, int32) error
}

var ErrBoundaryObserverPanic = errors.New("boundary observer panicked")

func routePhysicalAction(
	session *SourceSession,
	boundary BoundaryObserver,
	action control.InputAction,
	pointerX int32,
) bool {
	if session == nil {
		return false
	}
	// The physical down/up decision and grant activation share one gate. In
	// particular a local down observed during REQUEST cannot acquire a remote up.
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	session.observePhysicalPress(action)
	if boundary != nil {
		if err := observeBoundary(boundary, action, pointerX); err != nil {
			if errors.Is(err, ErrBoundaryObserverPanic) {
				session.Abort(err)
				return false
			}
			if !errors.Is(err, control.ErrBoundaryIntentQueueFull) && session.Active() {
				session.Abort(err)
			}
			return false
		}
	}
	return session.Offer(action)
}

func observeBoundary(boundary BoundaryObserver, action control.InputAction, pointerX int32) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrBoundaryObserverPanic
		}
	}()
	return boundary.Observe(action, pointerX)
}
