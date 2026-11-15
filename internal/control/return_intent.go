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
	if intent.SessionID == "" || intent.Epoch == "" || !intent.Height.Valid() {
		return fmt.Errorf("%w: session and epoch are required", ErrInvalidReturnIntent)
	}
	return nil
}
