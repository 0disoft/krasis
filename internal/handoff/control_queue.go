package handoff

import (
	"context"
	"time"
)

// ControlQueueProfile is the complete TLS-frame read to dispatcher dequeue,
// including bounded-channel backpressure. Existing exchange spans still start
// at dequeue, preserving their schema and historical comparison boundaries.
type ControlQueueProfile struct {
	Schema uint8  `json:"schema"`
	Micros uint32 `json:"micros"`
}

func (p ControlQueueProfile) Valid() bool {
	return p.Schema == 1 && p.Micros <= uint32(Window.Microseconds())
}

type controlQueueKey struct{}

func WithControlQueue(ctx context.Context, received, dequeued time.Time) context.Context {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() || received.IsZero() || dequeued.IsZero() ||
		dequeued.Before(received) || dequeued.Sub(received) > Window {
		return ctx
	}
	return context.WithValue(ctx, controlQueueKey{}, ControlQueueProfile{Schema: 1, Micros: uint32(dequeued.Sub(received).Microseconds())})
}

// Caller holds the collector lock and has matched the exact request/grant.
func recordControlQueue(ctx context.Context, a *attempt) {
	if a.summary.ControlQueue == nil {
		if p, ok := ctx.Value(controlQueueKey{}).(ControlQueueProfile); ok && p.Valid() {
			a.summary.ControlQueue = &p
		}
	}
}
