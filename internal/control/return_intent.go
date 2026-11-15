package control

import (
	"errors"
	"fmt"
)

var ErrInvalidReturnIntent = errors.New("invalid return intent")

type ReturnIntent struct {
	SessionID string
	Epoch     string
	Height    EdgeHeight
}

func (intent ReturnIntent) Validate() error {
	if intent.SessionID == "" || intent.Epoch == "" {
		return fmt.Errorf("%w: session and epoch are required", ErrInvalidReturnIntent)
	}
	if !intent.Height.Valid() {
		return fmt.Errorf("%w: invalid crossing height", ErrInvalidReturnIntent)
	}
	return nil
}
