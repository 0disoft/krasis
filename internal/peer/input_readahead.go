package peer

import (
	"context"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type queuedSessionFrame struct {
	frame         transport.Frame
	err           error
	controlReadAt time.Time
}

// One bounded reader owns wire order. Dispatch is still the sole owner of
// runtime, injection, control and heartbeat validation. Closing joins the reader.
type inputReadAhead struct {
	ctx        context.Context
	frames     chan queuedSessionFrame
	pending    queuedSessionFrame
	hasPending bool
	batch      [application.MaxPointerMoveBatch]control.InputEnvelope
	done       chan struct{}
}

func startInputReadAhead(ctx context.Context, connection SessionConnection) *inputReadAhead {
	r := &inputReadAhead{ctx: ctx, frames: make(chan queuedSessionFrame, application.MaxPointerMoveBatch), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		defer close(r.frames)
		for {
			frame, err := connection.ReadFrame()
			p := queuedSessionFrame{frame: frame, err: err}
			if err == nil && frame.Kind == transport.MessageControl {
				// Capture before the bounded channel can block. Dequeue time
				// alone misclassifies dispatch backlog as network wait.
				p.controlReadAt = time.Now()
			}
			select {
			case r.frames <- p:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return r
}

func (r *inputReadAhead) ReadFrame() (transport.Frame, error) {
	p := r.readFrame()
	return p.frame, p.err
}

func (r *inputReadAhead) readFrame() queuedSessionFrame {
	if r.hasPending {
		p := r.pending
		r.pending, r.hasPending = queuedSessionFrame{}, false
		return p
	}
	p, ok := <-r.frames
	if !ok {
		if err := r.ctx.Err(); err != nil {
			return queuedSessionFrame{err: err}
		}
		return queuedSessionFrame{err: io.EOF}
	}
	return p
}

// Combine only immediately available adjacent validated pointer frames. Never
// wait for a future move or cross a key, button, wheel, control, heartbeat, error,
// identity change, sequence discontinuity, zero sum or input delta bound.
// Envelopes borrow dispatch-owned storage until the next pointerBatch call.
// runSession consumes them synchronously; the reader goroutine never accesses it.
func (r *inputReadAhead) pointerBatch(first transport.Frame) ([]control.InputEnvelope, control.InputAction) {
	if first.Kind != transport.MessageInput {
		return nil, control.InputAction{}
	}
	envelope, move, err := transport.DecodeInputPayloadReusingIdentifiers(first.Payload, r.batch[0])
	if err != nil || move.Kind != control.InputPointerMove {
		return nil, control.InputAction{}
	}
	r.batch[0] = envelope
	envelopes := r.batch[:1]
	for len(envelopes) < application.MaxPointerMoveBatch {
		select {
		case p, ok := <-r.frames:
			if !ok {
				return envelopes, move
			}
			if p.err != nil || p.frame.Kind != transport.MessageInput {
				r.pending, r.hasPending = p, true
				return envelopes, move
			}
			last := envelopes[len(envelopes)-1]
			next, action, err := transport.DecodeInputPayloadReusingIdentifiers(p.frame.Payload, last)
			dx, dy := int64(move.DeltaX)+int64(action.DeltaX), int64(move.DeltaY)+int64(action.DeltaY)
			if err != nil || action.Kind != control.InputPointerMove || next.SessionID != last.SessionID || next.Epoch != last.Epoch || last.Sequence == ^uint64(0) || next.Sequence != last.Sequence+1 || dx < -control.MaxInputDelta || dx > control.MaxInputDelta || dy < -control.MaxInputDelta || dy > control.MaxInputDelta || dx == 0 && dy == 0 {
				r.pending, r.hasPending = p, true
				return envelopes, move
			}
			envelopes = append(envelopes, next)
			move.DeltaX, move.DeltaY = int32(dx), int32(dy)
		default:
			return envelopes, move
		}
	}
	return envelopes, move
}
