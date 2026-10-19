package handoff

import (
	"context"
	"testing"
	"time"
)

func TestRequestPredecessorKeepsOneLocalDispatchAndRead(t *testing.T) {
	_, ctx, out := collected(t)
	base := time.Now()
	read := WithControlRead(ctx, base.Add(86*time.Millisecond))
	read = WithReadPredecessor(read, 2, base, base.Add(80*time.Millisecond), base.Add(81*time.Millisecond))
	p := Begin(read, "receiver", "session", "epoch", base)
	RequestReceived(p, base.Add(87*time.Millisecond))
	// Grant write and a repeated request callback cannot replace the evidence.
	changed := WithReadPredecessor(p, 1, base, base, base)
	RequestReceived(changed, base.Add(88*time.Millisecond))
	GrantWritten(p, base.Add(90*time.Millisecond))
	Finish(p, "ok")
	want := ReadPredecessorProfile{Schema: 1, Kind: 2, Micros: [3]uint32{80000, 1000, 5000}}
	if got := (*out)[0].ReadPredecessor; got == nil || *got != want {
		t.Fatal("request did not retain the immediately preceding dispatch", got)
	}
}

func TestRequestPredecessorMissingReversedAndDisabledStayUnobserved(t *testing.T) {
	_, ctx, _ := collected(t)
	base := time.Now()
	read := WithControlRead(ctx, base.Add(3*time.Second))
	for _, c := range []context.Context{
		WithReadPredecessor(ctx, 1, base, base, base),  // missing read completion
		WithReadPredecessor(read, 0, base, base, base), // first frame
		WithReadPredecessor(read, 4, base, base, base), // disconnect has no successor
		WithReadPredecessor(read, 1, time.Time{}, base, base),
		WithReadPredecessor(read, 1, base, base.Add(-time.Second), base),
		WithReadPredecessor(read, 1, base, base.Add(time.Second), base),
		WithReadPredecessor(read, 1, base, base, base.Add(4*time.Second)),
	} {
		if c.Value(readPredecessorKey{}) != nil {
			t.Fatal("missing or reversed boundary produced a predecessor")
		}
	}
	plain := context.Background()
	if WithReadPredecessor(plain, 1, base, base, base) != plain {
		t.Fatal("disabled diagnostics allocated read context")
	}
}

func TestRequestPredecessorDoesNotAttachToSourceFinishedOrExpiredAttempt(t *testing.T) {
	for _, state := range []string{"source", "finished", "expired", "invalid_time"} {
		t.Run(state, func(t *testing.T) {
			c, ctx, _ := collected(t)
			base := time.Now()
			read := WithReadPredecessor(WithControlRead(ctx, base), 1, base, base, base)
			role := "receiver"
			if state == "source" {
				role = "source"
			}
			p := Begin(read, role, "s", "e", base)
			at := base
			switch state {
			case "finished":
				Finish(p, "ok")
			case "expired":
				c.started = time.Now().Add(-Window)
			case "invalid_time":
				at = base.Add(-time.Second)
			}
			RequestReceived(p, at)
			if getAttempt(p).summary.ReadPredecessor != nil {
				t.Fatal("invalid observation attached")
			}
		})
	}
}

func TestRequestPredecessorClampsAndValidatesRoleKindAndBounds(t *testing.T) {
	_, ctx, out := collected(t)
	base := time.Now()
	read := WithReadPredecessor(WithControlRead(ctx, base.Add(6*Window)), 3, base, base.Add(2*Window), base.Add(4*Window))
	p := Begin(read, "receiver", "s", "e", base)
	RequestReceived(p, base.Add(6*Window))
	Finish(p, "ok")
	got := *(*out)[0].ReadPredecessor
	limit := uint32(Window.Microseconds())
	if got.Micros != ([3]uint32{limit, limit, limit}) || !got.Valid("receiver") || got.Valid("source") {
		t.Fatal("local duration bounds or receiver-only scope lost", got)
	}
	for _, mutate := range []func(*ReadPredecessorProfile){
		func(p *ReadPredecessorProfile) { p.Schema = 2 },
		func(p *ReadPredecessorProfile) { p.Kind = 0 },
		func(p *ReadPredecessorProfile) { p.Kind = 4 },
		func(p *ReadPredecessorProfile) { p.Micros[1]++ },
	} {
		bad := got
		mutate(&bad)
		if bad.Valid("receiver") {
			t.Fatal("invalid evidence accepted", bad)
		}
	}
}
