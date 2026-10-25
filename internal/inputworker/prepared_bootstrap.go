package inputworker

import (
	"context"
	"errors"
	"io"
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
