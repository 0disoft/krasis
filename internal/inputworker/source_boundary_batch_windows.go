//go:build windows

package inputworker

import "time"

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
	if err := w.channel.authority.checkSource(); err != nil {
		return sourceTransportFailure(err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.channel.authority.checkSourceHeartbeat(); err != nil {
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
	if err := w.channel.stream.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return sourceTransportFailure(err)
	}
	w.sequence = next
	return sourceTransportFailure(writeFrame(w.channel.stream, wire[:len(batch)*sourcePacketSize]))
}
