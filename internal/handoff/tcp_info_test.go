package handoff

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestTCPInfoMatchesSourceGrantAndExportsOnlyCounterDifferences(t *testing.T) {
	_, ctx, out := collected(t)
	var calls int
	ConfigureTCPInfo(ctx, func() ([4]uint32, bool) {
		calls++
		if calls == 1 {
			return [4]uint32{300000, 5000, 1000, 5}, true
		}
		return [4]uint32{400000, 5000, 1200, 6}, true
	})
	p := Begin(ctx, "source", "s", "e", time.Now())
	Granted(Matching(WithControlRead(ctx, time.Now()), "source", "foreign", "e"))
	if calls != 1 {
		t.Fatal("foreign grant queried current source socket")
	}
	grant := Matching(WithControlRead(ctx, time.Now()), "source", "s", "e")
	Granted(grant)
	Granted(grant)
	Finish(p, "ok")
	got := (*out)[0].TCPInfo
	if calls != 2 || got == nil || !got.Valid("source") || got.RTTMicros != ([2]uint32{300000, 400000}) ||
		got.MinRTTMicros != ([2]uint32{5000, 5000}) || !got.CounterDeltasKnown || got.RetransmittedBytes != 200 || got.TimeoutEpisodes != 1 {
		t.Fatal("counter baseline, RTT or one-shot budget lost", got, calls)
	}
	Granted(grant)
	if calls != 2 {
		t.Fatal("finished attempt sampled again")
	}
}

func TestTCPInfoAbsentFailedAndWrappedQueriesCannotInventNoRetransmissions(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail int
		wrap bool
	}{{"before_missing", 1, false}, {"after_missing", 2, false}, {"wrap", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx, out := collected(t)
			calls := 0
			ConfigureTCPInfo(ctx, func() ([4]uint32, bool) {
				calls++
				count := uint32(calls)
				if tc.wrap {
					count = uint32(3 - calls)
				}
				return [4]uint32{100, 50, count, count}, calls != tc.fail
			})
			p := Begin(ctx, "source", "s", "e", time.Now())
			Granted(WithControlRead(p, time.Now()))
			Finish(p, "ok")
			got := (*out)[0].TCPInfo
			if tc.fail != 0 {
				if got != nil {
					t.Fatal("missing query became a complete pair", got)
				}
			} else if got == nil || got.CounterDeltasKnown || got.RetransmittedBytes != 0 || got.TimeoutEpisodes != 0 || !got.Valid("source") {
				t.Fatal("wrapped counters became false known deltas", got)
			}
		})
	}
}

func TestTCPInfoStaysWithinCollectorBudgetAndNeverQueriesReceiver(t *testing.T) {
	for _, role := range []string{"source", "receiver"} {
		_, ctx, out := collected(t)
		calls := 0
		ConfigureTCPInfo(ctx, func() ([4]uint32, bool) { calls++; return [4]uint32{100, 50, 0, 0}, true })
		for i := 0; i < Limit+2; i++ {
			p := Begin(ctx, role, "s", "e", time.Now())
			Granted(WithControlRead(p, time.Now()))
			Finish(p, "ok")
		}
		want := 0
		if role == "source" {
			want = 2 * Limit
		}
		if calls != want || len(*out) != Limit {
			t.Fatal("query escaped bounded source attempts", calls, want, len(*out))
		}
	}
	ConfigureTCPInfo(context.Background(), func() ([4]uint32, bool) { t.Fatal("disabled collector queried"); return [4]uint32{}, false })
	sampleTCPInfo(context.Background(), false)
}

func TestTCPInfoSlowQueryDoesNotHoldCollectorLockOrMutateFrozenResult(t *testing.T) {
	_, ctx, out := collected(t)
	var calls atomic.Uint32
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	ConfigureTCPInfo(ctx, func() ([4]uint32, bool) {
		if calls.Add(1) == 2 {
			close(entered)
			<-release
		}
		return [4]uint32{100, 50, 0, 0}, true
	})
	p := Begin(ctx, "source", "s", "e", time.Now())
	go func() { Granted(WithControlRead(p, time.Now())); close(finished) }()
	<-entered
	Finish(p, "error")
	close(release)
	<-finished
	if len(*out) != 1 || (*out)[0].TCPInfo != nil {
		t.Fatal("late statistics rewrote frozen summary")
	}
}

func TestTCPInfoValidityRejectsWrongRoleUnknownCountersAndUnboundedDurations(t *testing.T) {
	p := TCPInfoProfile{Schema: 1, RTTMicros: [2]uint32{100, 200}, MinRTTMicros: [2]uint32{50, 50}, CounterDeltasKnown: true}
	if !p.Valid("source") || p.Valid("receiver") {
		t.Fatal("source-only numeric profile rejected or receiver accepted")
	}
	for _, mutate := range []func(*TCPInfoProfile){
		func(p *TCPInfoProfile) { p.Schema = 2 },
		func(p *TCPInfoProfile) { p.RTTMicros[0] = 0 },
		func(p *TCPInfoProfile) { p.QueryMicros[0] = uint32(Window.Microseconds()) + 1 },
		func(p *TCPInfoProfile) { p.MinRTTMicros[1] = ^uint32(0) },
		func(p *TCPInfoProfile) { p.CounterDeltasKnown = false; p.RetransmittedBytes = 1 },
	} {
		bad := p
		mutate(&bad)
		if bad.Valid("source") {
			t.Fatal("invalid TCP statistics accepted", bad)
		}
	}
}
