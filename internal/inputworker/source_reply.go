package inputworker

import (
	"context"
	"time"
)

// A reply published before EOF/cancellation is still an observed outcome,
// including rejection or a protocol violation. Consume it exactly once.
func waitSourceReply(ctx context.Context, timeout <-chan time.Time, replies <-chan sourcePacket, done <-chan struct{}) (sourcePacket, bool) {
	p, known, _ := waitSourceReplyObserved(ctx, timeout, replies, done)
	return p, known
}

func waitSourceReplyObserved(ctx context.Context, timeout <-chan time.Time, replies <-chan sourcePacket, done <-chan struct{}) (sourcePacket, bool, string) {
	outcome := "reader_ended"
	select {
	case reply := <-replies:
		return reply, true, "received"
	case <-ctx.Done():
		outcome = "cancelled"
		if ctx.Err() == context.DeadlineExceeded {
			outcome = "timeout"
		}
	case <-timeout:
		outcome = "timeout"
	case <-done:
	}
	select {
	case reply := <-replies:
		return reply, true, "received"
	default:
		return sourcePacket{}, false, outcome
	}
}

func sourceReplyResult(request, reply sourcePacket) (sourcePacket, error) {
	if reply.kind != sourceReply || reply.reply != request.sequence || reply.op != request.op {
		return sourcePacket{}, ErrProtocol
	}
	if reply.status != 0 && !(request.op == sourceCursorPrepare && reply.status == sourceCursorUnavailable) {
		return reply, ErrAuthority
	}
	return reply, nil
}
