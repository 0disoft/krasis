package peer

import (
	"context"
	"io"
	"sync"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type queuedInputFrame struct {
	queuedSessionFrame
	first     control.InputEnvelope
	move      control.InputAction
	lastFrame transport.Frame
	lastMove  control.InputAction
	count     int
}

// Short memory-only ownership: the wire reader appends; dispatch pops. At most
// 64 entries, each retaining at most two payloads and 64 contiguous sequences.
// No native call, peer write or runtime transition executes under this mutex.
type inputFrameQueue struct {
	mu           sync.Mutex
	entries      [application.MaxPointerMoveBatch]queuedInputFrame
	head, size   int
	ready, space chan struct{}
	decodeHint   control.InputEnvelope // Sole wire reader owns the decoding hint.
}

func newInputFrameQueue() *inputFrameQueue {
	return &inputFrameQueue{ready: make(chan struct{}, 1), space: make(chan struct{}, 1)}
}

func (q *inputFrameQueue) prepare(p queuedSessionFrame) queuedInputFrame {
	item := queuedInputFrame{queuedSessionFrame: p}
	if p.err == nil && p.frame.Kind == transport.MessageInput {
		envelope, move, err := transport.DecodeInputPayloadReusingIdentifiers(p.frame.Payload, q.decodeHint)
		if err == nil && move.Kind == control.InputPointerMove {
			q.decodeHint = envelope
			item.first, item.move, item.lastFrame, item.lastMove, item.count = envelope, move, p.frame, move, 1
		}
	}
	return item
}

func (q *inputFrameQueue) push(item queuedInputFrame) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size > 0 && item.count == 1 {
		last := &q.entries[(q.head+q.size-1)%len(q.entries)]
		dx, dy := int64(last.move.DeltaX)+int64(item.move.DeltaX), int64(last.move.DeltaY)+int64(item.move.DeltaY)
		if last.count > 0 && last.count < application.MaxPointerMoveBatch && last.first.SessionID == item.first.SessionID && last.first.Epoch == item.first.Epoch && last.first.Sequence <= ^uint64(0)-uint64(last.count) && item.first.Sequence == last.first.Sequence+uint64(last.count) && dx >= -control.MaxInputDelta && dx <= control.MaxInputDelta && dy >= -control.MaxInputDelta && dy <= control.MaxInputDelta {
			last.count++
			last.move.DeltaX, last.move.DeltaY = int32(dx), int32(dy)
			last.lastFrame, last.lastMove = item.frame, item.move
			return true
		}
	}
	if q.size == len(q.entries) {
		return false
	}
	q.entries[(q.head+q.size)%len(q.entries)] = item
	q.size++
	select {
	case q.ready <- struct{}{}:
	default:
	}
	return true
}

func (q *inputFrameQueue) pop() (queuedInputFrame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size == 0 {
		return queuedInputFrame{}, false
	}
	item := q.entries[q.head]
	q.entries[q.head] = queuedInputFrame{}
	q.head, q.size = (q.head+1)%len(q.entries), q.size-1
	select {
	case q.space <- struct{}{}:
	default:
	}
	return item, true
}

func (r *inputReadAhead) queuedFrame() queuedSessionFrame {
	for {
		if err := context.Cause(r.ctx); err != nil {
			return queuedSessionFrame{err: err}
		}
		if item, ok := r.queue.pop(); ok {
			return r.prepareQueuedFrame(item)
		}
		select {
		case <-r.queue.ready:
		case <-r.ctx.Done():
		case <-r.done:
			if item, ok := r.queue.pop(); ok {
				return r.prepareQueuedFrame(item)
			}
			return queuedSessionFrame{err: io.EOF}
		}
	}
}

func (r *inputReadAhead) prepareQueuedFrame(item queuedInputFrame) queuedSessionFrame {
	r.preparedCount, r.preparedMove = item.count, item.move
	// A zero-sum run is represented by two nonzero moves, preserving
	// all sequences without submitting an invalid zero-delta action.
	if item.count > 1 && item.move.DeltaX == 0 && item.move.DeltaY == 0 {
		r.preparedCount--
		r.preparedMove.DeltaX, r.preparedMove.DeltaY = -item.lastMove.DeltaX, -item.lastMove.DeltaY
		r.pending, r.hasPending = queuedSessionFrame{frame: item.lastFrame}, true
	}
	for i := 0; i < r.preparedCount; i++ {
		r.batch[i] = item.first
		r.batch[i].Sequence += uint64(i)
	}
	return item.queuedSessionFrame
}
