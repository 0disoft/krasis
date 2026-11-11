package wininput

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/x11desktop"
)

// Cancellation can close the native connection after XTEST accepted a press.
// Only idempotent key/button UP operations may use a separate recovery owner.
// Keep the primary owner poisoned: Press/Move/Wheel must never resume on it.
func (p *linuxInjection) recoverRelease(ctx context.Context, token control.PressedToken, cause error) error {
	if cause == nil || ctx == nil || ctx.Err() != nil ||
		(!errors.Is(cause, net.ErrClosed) && !errors.Is(cause, io.EOF)) {
		return cause
	}
	if err := token.Validate(); err != nil {
		return errors.Join(cause, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return cause
	}
	if ctx.Err() != nil {
		return errors.Join(cause, ctx.Err())
	}
	if p.releaseDesktop == nil {
		var err error
		p.releaseDesktop, err = x11desktop.Open(ctx)
		if err != nil {
			return errors.Join(cause, err)
		}
	}
	var err error
	switch token.Kind {
	case control.PressedKeyboardKey:
		err = p.releaseDesktop.Key(ctx, token.Code, false)
	case control.PressedMouseButton:
		err = p.releaseDesktop.Button(ctx, token.Code, false)
	default:
		return errors.Join(cause, ErrUnsupportedInput)
	}
	if err != nil {
		return errors.Join(cause, err)
	}
	return nil
}
