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
	height ...control.EdgeHeight,
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
		var h control.EdgeHeight
		if len(height) != 0 {
			h = height[0]
		}
		if err := ObserveBoundaryHeight(boundary, action, pointerX, h); err != nil {
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
	return ObserveBoundaryHeight(boundary, action, pointerX, 0)
}

// ObserveBoundaryHeight preserves legacy observers and the panic boundary.
func ObserveBoundaryHeight(boundary BoundaryObserver, action control.InputAction, pointerX int32, height control.EdgeHeight) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrBoundaryObserverPanic
		}
	}()
	if positioned, ok := boundary.(interface {
		ObserveHeight(control.InputAction, int32, control.EdgeHeight) error
	}); ok {
		return positioned.ObserveHeight(action, pointerX, height)
	}
	return boundary.Observe(action, pointerX)
}
