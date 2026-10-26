//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"
)

// One existing receipt budget covers both its fresh proof and its bytes. A
// timed-out native query retains its slot and exact handles until retirement;
// the service's existing recovery policy still decides whether it can continue.
func acknowledgePreparation(ctx context.Context, channel *Channel) error {
	if ctx == nil || channel == nil || channel.authority == nil || channel.authority.kind != authorityAgent {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	finish := startupCancellation(ctx, channel)
	defer finish()
	if err := channel.authorizeInput(ctx, false); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	deadline, _ := ctx.Deadline()
	if err := channel.stream.SetWriteDeadline(deadline); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if err := writeFrame(channel.stream, preparationReceipt[:]); err != nil {
		return errors.Join(ErrAuthority, err, ctx.Err())
	}
	return finish()
}
