package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"
)

// Preparation has already authenticated and consumed its only request. The
// peer sends no more bytes before the receipt. Disconnect (or extra data) can
// cancel unbound construction, but this function still joins its cleanup.
func watchPreparationPeer(ctx context.Context, stream net.Conn, prepare func(context.Context) error) error {
	if ctx == nil || stream == nil || prepare == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	watch, cancelWatch := context.WithCancel(run)
	defer cancelWatch()
	var watching atomic.Bool
	watching.Store(true)
	done := make(chan struct{})
	wake := context.AfterFunc(run, func() { _ = stream.SetReadDeadline(time.Now()) })
	go func() {
		defer close(done)
		var extra [1]byte
		if reader, ok := stream.(interface {
			readContext(context.Context, []byte) (int, error)
		}); ok {
			_, _ = reader.readContext(watch, extra[:])
		} else {
			_, _ = stream.Read(extra[:])
		}
		if watching.CompareAndSwap(true, false) {
			cancel()
		}
	}()
	result := prepare(run)
	// Disarm before interrupting our own unused read. Successful preparation
	// must not cancel itself or retain a reader when the receipt is written.
	watching.Store(false)
	interrupt := stream.SetReadDeadline(time.Now())
	cancelWatch()
	if interrupt != nil {
		_ = stream.Close()
	}
	<-done
	wake()
	return errors.Join(result, run.Err(), interrupt)
}
