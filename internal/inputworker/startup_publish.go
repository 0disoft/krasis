package inputworker

import (
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type startupPublisher interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

// Failure remains failure even if the worker later supplies a clean recovery
// receipt. The caller still owns shutdown and consumption of that receipt.
func publishStartupReady(stream startupPublisher, authorized func(bool) error, profile handoff.StartupProfile) error {
	if err := authorized(false); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if err := stream.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if err := writeStartupReady(stream, profile); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if err := stream.SetWriteDeadline(time.Time{}); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	return nil
}
