package handoff

import (
	"context"
	"time"
)

// ReadPredecessorProfile describes the successful dispatch immediately before
// a receiver's request read. Kind is the fixed wire frame kind (1 control,
// 2 input, 3 heartbeat); no payload or absolute timestamp is retained.
// These local spans do not prove that a request was queued during dispatch.
type ReadPredecessorProfile struct {
	Schema uint8     `json:"schema"`
	Kind   uint8     `json:"kind"`
	Micros [3]uint32 `json:"micros"` // dispatch, dispatch-end to read-start, read
}

func (p ReadPredecessorProfile) Valid(role string) bool {
	if role != "receiver" || p.Schema != 1 || p.Kind < 1 || p.Kind > 3 {
		return false
	}
	for _, n := range p.Micros {
		if n > uint32(Window.Microseconds()) {
			return false
		}
	}
	return true
}

type readPredecessorKey struct{}

// WithReadPredecessor captures only the preceding frame, before request
// dispatch can mutate session state. A first frame or a missing/reversed local
// boundary stays unobserved. It never changes read order or protocol decisions.
func WithReadPredecessor(ctx context.Context, kind uint8, started, finished, readStarted time.Time) context.Context {
	s := getSession(ctx)
	readFinished := controlReadAt(ctx)
	if s == nil || s.collector.stopped.Load() || kind < 1 || kind > 3 || started.IsZero() ||
		finished.Before(started) || readStarted.Before(finished) || readFinished.Before(readStarted) {
		return ctx
	}
	p := ReadPredecessorProfile{Schema: 1, Kind: kind}
	for i, elapsed := range [3]time.Duration{finished.Sub(started), readStarted.Sub(finished), readFinished.Sub(readStarted)} {
		p.Micros[i] = uint32(min(elapsed, Window).Microseconds())
	}
	return context.WithValue(ctx, readPredecessorKey{}, p)
}
