//go:build windows

package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/control"
)

// Suppression starts before native activation, but its queued input must not
// compete with activation's full proofs or precede the actual completion ACK.
// The serial command owner alone publishes/finishes this one-epoch fence.
type sourceActivationFence struct {
	epoch    uint64
	done     chan struct{}
	accepted bool // Published by closing done; never read before that close.
}

func (w *sourceWriter) beginActivation(epoch uint64) *sourceActivationFence {
	fence := &sourceActivationFence{epoch: epoch, done: make(chan struct{})}
	w.activation.Store(fence)
	return fence
}

func finishSourceActivation(fence *sourceActivationFence, accepted bool) {
	if fence != nil {
		fence.accepted = accepted
		close(fence.done)
	}
}

func (w *sourceWriter) completeSourceCommand(request, response sourcePacket, operation error, fence *sourceActivationFence) error {
	failure := sourceCommandCompletion(operation, w.sendCompletedReply(request, response))
	// A safely rejected command can keep the owner alive, but its negative ACK
	// must never release queued input as a successful activation.
	finishSourceActivation(fence, operation == nil && failure == nil)
	return failure
}

func (w *sourceWriter) awaitActivation(ctx context.Context, current func() bool) error {
	fence := w.activation.Load()
	if fence == nil {
		return nil
	}
	var stopped <-chan struct{}
	if w.ctx != nil {
		stopped = w.ctx.Done()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-stopped:
		return w.ctx.Err()
	case <-fence.done:
	}
	if !fence.accepted || w.activation.Load() != fence || w.epoch.Load() != fence.epoch || current != nil && !current() {
		return control.ErrSourceActionDiscarded
	}
	// SendAction still obtains its fresh full authority and repeats current at
	// wire admission. Finishing this fence is ordering, never an authority proof.
	return nil
}
