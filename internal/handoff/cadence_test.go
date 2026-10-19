package handoff

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInputCadenceCountsIntervalsAndKeepsOneMatchingMaximum(t *testing.T) {
	var c inputCadence
	at := time.Unix(100, 0)
	v := &sample{started: at}
	c.start(v)
	if c.snapshot() != nil {
		t.Fatal("first input fabricated an interval")
	}
	for _, gap := range []time.Duration{0, time.Microsecond, time.Millisecond, 4 * time.Millisecond, 10 * time.Millisecond, 25 * time.Millisecond, 100 * time.Millisecond} {
		c.complete(v, at.Add(gap/4))
		at = at.Add(gap)
		v = &sample{started: at}
		c.start(v)
	}
	p := c.snapshot()
	if p == nil || !p.Valid() || p.Intervals != 7 || p.Histogram != [6]uint32{2, 1, 1, 1, 1, 1} ||
		p.MaxGapMicros != 100000 || p.PreviousDispatchMicros != 25000 || p.BetweenDispatchMicros != 75000 {
		t.Fatalf("incorrect interval accounting or mixed maximum: %+v", p)
	}
	c.complete(v, at)
	c.start(&sample{started: at.Add(time.Millisecond)})
	if p.Intervals != 7 {
		t.Fatal("later input rewrote a frozen snapshot")
	}
}

func TestInputCadenceOverlappingOrLateCompletionCannotCompleteAnotherSample(t *testing.T) {
	var c inputCadence
	at := time.Unix(100, 0)
	old, next := &sample{started: at}, &sample{started: at}
	c.start(old)
	c.start(next) // Even an equal timestamp cannot identify the new sample.
	c.complete(old, at.Add(2*time.Millisecond))
	if !c.completed.IsZero() {
		t.Fatal("late completion was attributed to a different input")
	}
	c.complete(next, at.Add(time.Millisecond))
	c.complete(next, at.Add(15*time.Millisecond)) // Duplicate cannot extend it.
	c.start(&sample{started: at.Add(20 * time.Millisecond)})
	p := c.snapshot()
	if p == nil || !p.Valid() || p.Skipped != 1 || p.Intervals != 1 || p.PreviousDispatchMicros != 1000 || p.BetweenDispatchMicros != 19000 {
		t.Fatalf("overlap was not kept separate from the observed interval: %+v", p)
	}
}

func TestInputCadenceRejectsUnobservedReversedAndOutOfWindowIntervals(t *testing.T) {
	at := time.Unix(100, 0)
	for _, tc := range []struct {
		name     string
		complete time.Time
		next     time.Time
	}{
		{"unfinished", time.Time{}, at.Add(time.Millisecond)},
		{"reversed_completion", at.Add(-time.Millisecond), at.Add(time.Millisecond)},
		{"reversed_start", at.Add(time.Millisecond), at},
		{"outside_window", at, at.Add(Window + time.Nanosecond)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c inputCadence
			v := &sample{started: at}
			c.start(v)
			if !tc.complete.IsZero() {
				c.complete(v, tc.complete)
			}
			c.start(&sample{started: tc.next})
			p := c.snapshot()
			if p == nil || !p.Valid() || p.Skipped != 1 || p.Intervals != 0 || p.MaxGapMicros != 0 {
				t.Fatalf("unknown timing became an observed zero: %+v", p)
			}
		})
	}
}

func TestInputCadenceSaturatesWithoutWrappingAndValidatesTheEnvelope(t *testing.T) {
	at := time.Unix(100, 0)
	v := &sample{started: at}
	c := inputCadence{profile: CadenceProfile{Intervals: ^uint32(0), Histogram: [6]uint32{0, ^uint32(0)}, MaxGapMicros: 1000, BetweenDispatchMicros: 1000}}
	c.start(v)
	c.complete(v, at)
	c.start(&sample{started: at.Add(time.Millisecond)})
	p := c.snapshot()
	if !p.Valid() || !p.Saturated || p.Intervals != ^uint32(0) || p.Histogram[1] != ^uint32(0) {
		t.Fatal("counter saturation wrapped or invalidated the profile", p)
	}
	for _, mutate := range []func(*CadenceProfile){
		func(p *CadenceProfile) { p.Schema = 2 },
		func(p *CadenceProfile) { p.Intervals-- },
		func(p *CadenceProfile) { p.MaxGapMicros = uint32(Window.Microseconds()) + 1 },
		func(p *CadenceProfile) { p.PreviousDispatchMicros = 1001 },
		func(p *CadenceProfile) { p.BetweenDispatchMicros = 0 },
		func(p *CadenceProfile) { p.Histogram[1]--; p.Histogram[5]++ },
	} {
		bad := *p
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("contradictory cadence passed export validation", bad)
		}
	}
}

func TestInputCadenceUsesTheRetainedAttemptAndFreezesAtRetirement(t *testing.T) {
	for _, role := range []string{"source", "receiver"} {
		_, ctx, out := collected(t)
		attempt := Begin(ctx, role, "session", "epoch", time.Now())
		_, done := Input(ctx, role)
		done(nil)
		_, late := Input(ctx, role)
		Finish(attempt, "ok")
		p := (*out)[0].Cadence
		if p == nil || !p.Valid() || p.Intervals != 1 {
			t.Fatal("retirement lost retained cadence", p)
		}
		frozen := *p
		late(nil)
		_, ignored := Input(attempt, role)
		ignored(nil)
		if *p != frozen {
			t.Fatal("late input modified a retired profile")
		}
		next := Begin(ctx, role, "next", "epoch", time.Now())
		Finish(next, "ok")
		if (*out)[1].Cadence != nil {
			t.Fatal("new attempt inherited an old interval")
		}
	}
	_, noop := Input(context.Background(), "source")
	noop(nil)
}

func TestInputCadenceAddsNoPerFrameAllocations(t *testing.T) {
	var c inputCadence
	v := sample{started: time.Unix(100, 0)}
	if allocations := testing.AllocsPerRun(1000, func() {
		c.start(&v)
		c.complete(&v, v.started.Add(time.Microsecond))
		v.started = v.started.Add(4 * time.Millisecond)
	}); allocations != 0 {
		t.Fatal("cadence adds per-frame allocations", allocations)
	}
}

func TestInputCadenceRetirementRacesWithInFlightCompletions(t *testing.T) {
	_, ctx, out := collected(t)
	attempt := Begin(ctx, "receiver", "session", "epoch", time.Now())
	var done []func(error)
	for range 8 {
		_, complete := Input(ctx, "receiver")
		done = append(done, complete)
	}
	gate := make(chan struct{})
	var workers sync.WaitGroup
	for _, complete := range done {
		workers.Go(func() { <-gate; complete(nil) })
	}
	close(gate)
	Finish(attempt, "ok")
	p := (*out)[0].Cadence
	if p == nil || !p.Valid() || p.Skipped != 7 {
		t.Fatal("overlapping starts lost their unknown-boundary accounting", p)
	}
	frozen := *p
	workers.Wait()
	if *p != frozen {
		t.Fatal("concurrent completion rewrote the exported cadence")
	}
}
