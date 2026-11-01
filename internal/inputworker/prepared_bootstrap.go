package inputworker

import (
	"context"
	"errors"
	"io"
	"time"
)

type preparedBootstrapReader struct {
	ctx  context.Context
	read func(context.Context, []byte) (int, error)
}

func (r preparedBootstrapReader) Read(data []byte) (int, error) { return r.read(r.ctx, data) }

// After authentication, cancel only the exact bootstrap read, not the duplex
// pipe that must still carry retirement. The native reader drains cancellation
// and preserves a completed kernel read. Real leases still check startup.Err
// and service authority before binding; only a complete zero frame can retire.
func readPreparedBootstrap(startup context.Context, stopControl func() bool, read func(context.Context, []byte) (int, error)) (packet [bootstrapSize]byte, err error) {
	if startup == nil || stopControl == nil || read == nil {
		return packet, ErrAuthority
	}
	if !stopControl() || startup.Err() != nil {
		return packet, errors.Join(ErrAuthority, startup.Err())
	}
	_, err = io.ReadFull(preparedBootstrapReader{startup, read}, packet[:])
	return packet, err
}

// An authenticated, unbound reservation may outlive process startup. Read only
// its first byte on the idle budget, then start a fresh bounded bootstrap phase.
// No complete bootstrap or input authority is published by this first-byte read.
func readPreparedLeaseBootstrap(startup, idle context.Context, stopControl func() bool, read func(context.Context, []byte) (int, error), begin func() (context.Context, error)) (packet [bootstrapSize]byte, err error) {
	if startup == nil || idle == nil || stopControl == nil || read == nil || begin == nil {
		return packet, ErrAuthority
	}
	if !stopControl() || preparedBootstrapContextError(startup) != nil || preparedBootstrapContextError(idle) != nil {
		return packet, errors.Join(ErrAuthority, preparedBootstrapContextError(startup), preparedBootstrapContextError(idle))
	}
	if _, err = io.ReadFull(preparedBootstrapReader{idle, read}, packet[:1]); err != nil {
		return packet, err
	}
	if err = preparedBootstrapContextError(idle); err != nil {
		return packet, errors.Join(ErrAuthority, err)
	}
	binding, err := begin()
	if err != nil || binding == nil {
		return packet, errors.Join(ErrAuthority, err)
	}
	if err = preparedBootstrapContextError(binding); err != nil {
		return packet, errors.Join(ErrAuthority, err)
	}
	_, err = io.ReadFull(preparedBootstrapReader{binding, read}, packet[1:])
	if err == nil {
		err = preparedBootstrapContextError(binding)
	}
	return packet, err
}

// A completed kernel read can race delivery of a context's timer callback.
// The absolute deadline still fences admission at the exact boundary.
func preparedBootstrapContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}
