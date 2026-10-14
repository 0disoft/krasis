package inputworker

import (
	"context"
	"time"
)

// A reply published before EOF/cancellation is still an observed outcome,
// including rejection or a protocol violation. Consume it exactly once.
func waitSourceReply(ctx context.Context, timeout <-chan time.Time, replies <-chan sourcePacket, done <-chan struct{}) (sourcePacket, bool) {
	select {
	case reply := <-replies:
		return reply, true
	case <-ctx.Done():
	case <-timeout:
	case <-done:
	}
	select {
	case reply := <-replies:
		return reply, true
	default:
		return sourcePacket{}, false
	}
}

func sourceReplyResult(request, reply sourcePacket) (sourcePacket, error) {
	if reply.kind != sourceReply || reply.reply != request.sequence || reply.op != request.op {
		return sourcePacket{}, ErrProtocol
	}
	if reply.status != 0 {
		return reply, ErrAuthority
	}
	return reply, nil
}
