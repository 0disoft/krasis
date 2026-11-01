package inputworker

import (
	"context"
	"sync/atomic"
)

// One unacknowledged advisory batch reserves at most 1KiB of the 4KiB pipe.
// Waiting for the reader owns no wire gate, proof or write deadline. Command
// replies/retirement retain their independent space and existing ordering.
type sourceBoundaryWindow struct {
	permit  chan struct{}
	pending atomic.Pointer[sourceBoundaryReservation]
}

type sourceBoundaryReservation struct{ sequence atomic.Uint64 }

func newSourceBoundaryWindow() *sourceBoundaryWindow {
	w := &sourceBoundaryWindow{permit: make(chan struct{}, 1)}
	w.permit <- struct{}{}
	return w
}

func (w *sourceBoundaryWindow) acquire(ctx context.Context) (*sourceBoundaryReservation, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.permit:
	}
	r := &sourceBoundaryReservation{}
	w.pending.Store(r)
	if err := ctx.Err(); err != nil {
		w.release(r)
		return nil, err
	}
	return r, nil
}

func (w *sourceBoundaryWindow) release(r *sourceBoundaryReservation) bool {
	if !w.pending.CompareAndSwap(r, nil) {
		return false
	}
	w.permit <- struct{}{}
	return true
}

func (w *sourceBoundaryWindow) acknowledge(sequence uint64) error {
	r := w.pending.Load()
	if r == nil || sequence == 0 || r.sequence.Load() != sequence || !w.release(r) {
		return ErrProtocol
	}
	return nil
}
