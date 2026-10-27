package handoff

import (
	"context"
	"testing"
	"time"
)

func TestControlQueueMatchesRequestOrGrantAndKeepsExchangeBoundary(t *testing.T) {
	for _, role := range []string{"source", "receiver"} {
		t.Run(role, func(t *testing.T) {
			_, ctx, out := collected(t)
			base := time.Now()
			p := Begin(ctx, role, "session", "epoch", base)
			read := WithControlQueue(WithControlRead(ctx, base.Add(90*time.Millisecond)), base.Add(10*time.Millisecond), base.Add(90*time.Millisecond))
			matched := Matching(read, role, "session", "epoch")
			if role == "source" {
				RequestWritten(p, base)
				recordGrantRead(matched, base.Add(92*time.Millisecond))
			} else {
				RequestReceived(matched, base.Add(92*time.Millisecond))
				GrantWritten(matched, base.Add(100*time.Millisecond))
			}
			Finish(p, "ok")
			got := (*out)[0]
			if got.ControlQueue == nil || got.ControlQueue.Micros != 80000 || !got.ControlQueue.Valid() {
				t.Fatal("queue lost", got.ControlQueue)
			}
			stage, want := RequestWrittenToGrantRead, uint32(90000)
			if role == "receiver" {
				stage, want = RequestReadToGrantWritten, 10000
			}
			if got.Exchange.Micros[stage] != want {
				t.Fatal("historical boundary changed", got.Exchange)
			}
		})
	}
}

func TestControlQueueRejectsMissingReversedExpiredAndStaleEvidence(t *testing.T) {
	_, ctx, _ := collected(t)
	base := time.Now()
	for _, times := range [][2]time.Time{{{}, base}, {base, {}}, {base, base.Add(-time.Nanosecond)}, {base, base.Add(Window + time.Nanosecond)}} {
		if WithControlQueue(ctx, times[0], times[1]) != ctx {
			t.Fatal("invalid span accepted")
		}
	}
	plain := context.Background()
	if WithControlQueue(plain, base, base) != plain {
		t.Fatal("disabled collector allocated context")
	}
	p := Begin(ctx, "source", "session", "epoch", base)
	read := WithControlQueue(WithControlRead(ctx, base), base, base)
	recordGrantRead(Matching(read, "source", "old", "epoch"), base)
	if getAttempt(p).summary.ControlQueue != nil {
		t.Fatal("stale grant attached")
	}
	Finish(p, "ok")
	recordGrantRead(Matching(read, "source", "session", "epoch"), base)
	if getAttempt(p).summary.ControlQueue != nil {
		t.Fatal("retired attempt mutated")
	}
}
