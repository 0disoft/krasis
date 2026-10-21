//go:build windows

package inputworker

import (
	"context"
	"errors"
	"os"
)

const sourceBoundaryBatchLimit = 16

// Drain only notifications already waiting. There is no batching timer, lost
// edge/rearm sample or change to the remote action sender. Pending commands
// stop collection without being consumed or overtaken by more queue work.
func collectSourceBoundaries(first sourcePacket, events <-chan sourcePacket, commands <-chan sourcePacket, batch *[sourceBoundaryBatchLimit]sourcePacket) int {
	batch[0] = first
	n := 1
	for n < len(batch) && len(commands) == 0 {
		select {
		case p, ok := <-events:
			if !ok {
				return n
			}
			batch[n] = p
			n++
		default:
			return n
		}
	}
	return n
}

// One fresh authority check covers one bounded pipe write (at most 1KiB).
// Every original packet keeps its own binding and consecutive sequence. Input,
// command replies and retirement never enter this advisory-boundary batch.
func (w *sourceWriter) sendBoundaries(batch []sourcePacket) error {
	if len(batch) == 0 || len(batch) > sourceBoundaryBatchLimit {
		return ErrProtocol
	}
	if !w.started.Load() {
		return nil
	}
	// Slow SCM/WTS validation of advisory pointer observations must not hold
	// the shared writer gate needed by heartbeat replies. No input/actions or
	// capture decisions enter this batch. Validate fully before admission, then
	// recheck the pinned owner's presence under the sequence/write gate.
	ctx, cancel := w.operation(nil, false)
	defer cancel()
	if err := w.lockAuthorized(ctx, true); err != nil {
		if ctx.Err() == context.DeadlineExceeded && errors.Is(err, context.DeadlineExceeded) && (w.ctx == nil || w.ctx.Err() == nil) {
			// No gate or bytes were acquired. Drop this stale observation just
			// like queue overflow, retaining any native query until it joins.
			// Actual rejection, owner cancellation and attempted writes fail.
			return nil
		}
		return sourceTransportFailure(err)
	}
	defer w.mu.Unlock()
	if err := w.channel.authorizeSourcePresence(ctx); err != nil {
		return sourceTransportFailure(err)
	}
	var wire [sourceBoundaryBatchLimit * sourcePacketSize]byte
	next := w.sequence
	for i, p := range batch {
		if p.kind != sourceBoundary {
			return ErrProtocol
		}
		next++
		p.sequence, p.binding = next, w.binding
		data, err := encodeSourcePacket(p)
		if err != nil {
			return err
		}
		copy(wire[i*sourcePacketSize:], data[:])
	}
	err := w.writeSequenced(ctx, wire[:len(batch)*sourcePacketSize], next)
	var failure *sourceIOFailure
	if errors.As(err, &failure) && failure.unsubmitted && errors.Is(failure, os.ErrDeadlineExceeded) {
		// These are advisory pointer samples, just like the bounded queue's
		// overflow drops. A native pre-submission proof means no bytes or
		// sequence escaped. Discard this stale batch instead of canceling the
		// capture owner and its heartbeat reader. Never replay the batch, and
		// never suppress an attempted/partial write or an authority failure.
		return nil
	}
	return err
}
