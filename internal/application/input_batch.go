package application

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const MaxPointerMoveBatch = 64

// HandlePointerMoveBatch consumes every original, ordered envelope before one
// bounded pointer injection. Callers may combine only already decoded adjacent
// pointer moves, never cross a discrete/control frame, or omit an envelope.
// Success renews liveness only after injection, just like a single input.
func (runtime *SessionRuntime) HandlePointerMoveBatch(ctx context.Context, envelopes []control.InputEnvelope, move control.InputAction, now func() time.Time, timeout time.Duration) error {
	if len(envelopes) == 0 || len(envelopes) > MaxPointerMoveBatch || move.Kind != control.InputPointerMove || move.Validate() != nil || now == nil || timeout <= 0 {
		return errors.New("invalid pointer move batch")
	}
	return runtime.handleInputActions(ctx, envelopes, move, now, timeout)
}
