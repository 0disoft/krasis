package handoff

import (
	"context"
	"testing"
	"time"
)

func TestExchangeSourceAssociatesEndpointsInEitherCallbackOrder(t *testing.T) {
	for _, readFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "write_first", true: "read_first"}[readFirst], func(t *testing.T) {
			_, ctx, out := collected(t)
			p := Begin(ctx, "source", "session", "epoch", time.Now())
			base := getAttempt(p).request
			read := WithControlRead(ctx, base.Add(15*time.Millisecond))
			matching := Matching(read, "source", "session", "epoch")
			write := func() { RequestWritten(p, base.Add(3*time.Millisecond)) }
			grant := func() { recordGrantRead(matching, base.Add(18*time.Millisecond)) }
			if readFirst {
				grant()
				write()
			} else {
				write()
				grant()
			}
			RequestWritten(p, base.Add(4*time.Millisecond))
			recordGrantRead(WithControlRead(matching, base.Add(16*time.Millisecond)), base.Add(25*time.Millisecond))
			Finish(p, "ok")
			got := (*out)[0].Exchange
			if got == nil || got.Measured != 3 || got.Micros != ([ExchangeStageCount]uint32{12000, 3000}) {
				t.Fatalf("endpoints lost or overwritten: %+v", got)
			}
		})
	}
}

func TestExchangeOverlapIsAbsentRatherThanFalseZero(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "session", "epoch", time.Now())
	base := getAttempt(p).request
	RequestWritten(p, base.Add(10*time.Millisecond))
	recordGrantRead(WithControlRead(p, base.Add(9*time.Millisecond)), base.Add(12*time.Millisecond))
	Finish(p, "ok")
	got := (*out)[0].Exchange
	if got == nil || got.Measured != 1<<GrantReadToHandler || got.Micros[RequestWrittenToGrantRead] != 0 {
		t.Fatal("write/read overlap represented as a wait", got)
	}
}

func TestExchangeReceiverUsesReadContextAndClampsElapsed(t *testing.T) {
	_, ctx, out := collected(t)
	base := time.Now()
	p := Begin(WithControlRead(ctx, base), "receiver", "session", "epoch", base)
	RequestReceived(p, base)
	GrantWritten(p, base.Add(2*Window))
	Finish(p, "ok")
	got := (*out)[0].Exchange
	if got == nil || got.Measured != 12 || got.Micros[RequestReadToHandler] != 0 || got.Micros[RequestReadToGrantWritten] != uint32(Window.Microseconds()) {
		t.Fatal("observed zero, presence or clamp lost", got)
	}
}

func TestExchangeDoesNotAttachUnobservedStaleRetiredOrExpiredSamples(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "source", "session", "epoch", time.Now())
	base := getAttempt(p).request
	RequestWritten(p, base)
	recordGrantRead(p, base.Add(time.Second)) // direct caller without a read observation
	stale := Matching(WithControlRead(ctx, base), "source", "old", "epoch")
	recordGrantRead(stale, base.Add(time.Second))
	recordGrantRead(WithControlRead(p, base.Add(-time.Second)), base)
	RequestReceived(WithControlRead(p, base), base) // wrong role
	if getAttempt(p).summary.Exchange != nil {
		t.Fatal("unobserved, stale or wrong-role exchange attached")
	}
	Finish(p, "ok")
	recordGrantRead(WithControlRead(p, base), base)
	if (*out)[0].Exchange != nil {
		t.Fatal("retired summary changed")
	}
	p = Begin(WithControlRead(ctx, base), "receiver", "next", "epoch", base)
	c.started = time.Now().Add(-Window)
	RequestReceived(p, base)
	GrantWritten(p, base)
	if getAttempt(p).summary.Exchange != nil {
		t.Fatal("expired window gained a sample")
	}
	EndSession(ctx)
}

func TestExchangeAttemptLimitAndAbsentCollectorRemainNoops(t *testing.T) {
	_, ctx, _ := collected(t)
	for i := 0; i < Limit; i++ {
		p := Begin(ctx, "receiver", "session", "epoch", time.Now())
		Finish(p, "ok")
	}
	p := Begin(WithControlRead(ctx, time.Now()), "receiver", "session", "epoch", time.Now())
	RequestReceived(p, time.Now())
	GrantWritten(p, time.Now())
	if getAttempt(p) != nil {
		t.Fatal("exhausted collector reused a slot")
	}
	plain := context.Background()
	if WithControlRead(plain, time.Now()) != plain {
		t.Fatal("disabled collector added read context")
	}
	RequestWritten(plain, time.Now())
	RequestReceived(plain, time.Now())
	GrantWritten(plain, time.Now())
}

func TestExchangeRejectsInvalidPresenceBoundsAndRole(t *testing.T) {
	p := ExchangeProfile{Schema: 1, Measured: 1, Micros: [ExchangeStageCount]uint32{1000}}
	if !p.Valid("source") || p.Valid("receiver") {
		t.Fatal("role-specific mask ignored")
	}
	for _, mutate := range []func(*ExchangeProfile){
		func(p *ExchangeProfile) { p.Schema = 2 },
		func(p *ExchangeProfile) { p.Measured = 0 },
		func(p *ExchangeProfile) { p.Measured |= 16 },
		func(p *ExchangeProfile) { p.Micros[GrantReadToHandler] = 1 },
		func(p *ExchangeProfile) { p.Micros[RequestWrittenToGrantRead] = uint32(Window.Microseconds()) + 1 },
	} {
		bad := p
		mutate(&bad)
		if bad.Valid("source") {
			t.Fatal("invalid exchange accepted", bad)
		}
	}
}
