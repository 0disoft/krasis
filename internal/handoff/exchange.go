package handoff

import (
	"context"
	"time"
)

// ExchangeStage contains process-local control timings, not wire latency.
// Existing preparation/input vector indices remain unchanged.
type ExchangeStage uint8

const (
	RequestWrittenToGrantRead ExchangeStage = iota
	GrantReadToHandler
	RequestReadToHandler
	RequestReadToGrantWritten
	ExchangeStageCount
)

type ExchangeProfile struct {
	Schema   uint8                      `json:"schema"`
	Micros   [ExchangeStageCount]uint32 `json:"micros"`
	Measured uint8                      `json:"measured"`
}

func (p ExchangeProfile) Valid(role string) bool {
	allowed := uint8(3)
	if role == "receiver" {
		allowed = 12
	} else if role != "source" {
		return false
	}
	if p.Schema != 1 || p.Measured == 0 || p.Measured&^allowed != 0 {
		return false
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || (p.Measured&(1<<i) == 0 && n != 0) {
			return false
		}
	}
	return true
}

type controlReadKey struct{}

// WithControlRead is called immediately after a complete control frame is read,
// before progress observers, the policy clock or payload dispatch. The timestamp
// is local context only; it is never sent or logged and never used for authority.
func WithControlRead(ctx context.Context, at time.Time) context.Context {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() || at.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, controlReadKey{}, at)
}

func controlReadAt(ctx context.Context) time.Time {
	if ctx == nil {
		return time.Time{}
	}
	at, _ := ctx.Value(controlReadKey{}).(time.Time)
	return at
}

// RequestWritten records successful local write completion. A fast grant can
// reach the reader before this callback; both endpoints are retained once so
// callback ordering alone cannot lose an otherwise valid span.
func RequestWritten(ctx context.Context, at time.Time) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || at.IsZero() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !exchangeActive(s, a) || a.summary.Role != "source" || !a.requestWritten.IsZero() || at.Before(a.request) {
		return
	}
	a.requestWritten = at
	recordGrantWait(a)
}

func recordGrantRead(ctx context.Context, handledAt time.Time) {
	s, a := getSession(ctx), getAttempt(ctx)
	readAt := controlReadAt(ctx)
	if s == nil || a == nil || readAt.IsZero() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !exchangeActive(s, a) || a.summary.Role != "source" || !a.grantRead.IsZero() || readAt.Before(a.request) || readAt.After(handledAt) {
		return
	}
	a.grantRead = readAt
	putExchange(a, GrantReadToHandler, handledAt.Sub(readAt))
	recordGrantWait(a)
}

func recordGrantWait(a *attempt) {
	if !a.requestWritten.IsZero() && !a.grantRead.IsZero() {
		// A grant may actually be read before the write returns. That overlap
		// is unmeasured here, not a zero-duration wait or a negative sample.
		putExchange(a, RequestWrittenToGrantRead, a.grantRead.Sub(a.requestWritten))
	}
}

func RequestReceived(ctx context.Context, at time.Time) {
	recordReceiverExchange(ctx, RequestReadToHandler, at)
}

func GrantWritten(ctx context.Context, at time.Time) {
	recordReceiverExchange(ctx, RequestReadToGrantWritten, at)
}

func recordReceiverExchange(ctx context.Context, stage ExchangeStage, at time.Time) {
	s, a := getSession(ctx), getAttempt(ctx)
	readAt := controlReadAt(ctx)
	if s == nil || a == nil || readAt.IsZero() || at.IsZero() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if exchangeActive(s, a) && a.summary.Role == "receiver" {
		putExchange(a, stage, at.Sub(readAt))
	}
}

func exchangeActive(s *session, a *attempt) bool {
	return !a.finished && !s.collector.stopped.Load() && time.Since(s.collector.started) < Window
}

func putExchange(a *attempt, stage ExchangeStage, elapsed time.Duration) {
	if elapsed < 0 || stage >= ExchangeStageCount {
		return
	}
	if a.summary.Exchange == nil {
		a.summary.Exchange = &ExchangeProfile{Schema: 1}
	}
	p := a.summary.Exchange
	if p.Measured&(1<<stage) != 0 {
		return
	}
	elapsed = min(elapsed, Window)
	p.Micros[stage] = uint32(elapsed.Microseconds())
	p.Measured |= 1 << stage
}
