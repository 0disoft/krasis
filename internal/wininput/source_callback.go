package wininput

import (
	"errors"

	"github.com/0disoft/krasis/internal/control"
)

var ErrSourceMapperPanic = errors.New("source mapper panicked")

type sourceActionMapper func() (control.InputAction, bool, int32, error)

func routeMappedPhysicalAction(
	session *SourceSession,
	boundary BoundaryObserver,
	mapper sourceActionMapper,
) bool {
	action, emitted, pointerX, err := mapSourceAction(mapper)
	if err != nil {
		if session != nil && (errors.Is(err, ErrSourceMapperPanic) || session.Active()) {
			session.Abort(err)
		}
		return false
	}
	return emitted && routePhysicalAction(session, boundary, action, pointerX)
}

func mapSourceAction(mapper sourceActionMapper) (action control.InputAction, emitted bool, pointerX int32, err error) {
	if mapper == nil {
		return control.InputAction{}, false, 0, errors.New("source mapper is required")
	}
	defer func() {
		if recover() != nil {
			action = control.InputAction{}
			emitted = false
			pointerX = 0
			err = ErrSourceMapperPanic
		}
	}()
	return mapper()
}
