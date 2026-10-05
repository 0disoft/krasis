package handoff

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestGrantProgressPartitionsLocalPhasesAndFreezesMatchingAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	StartGrantProgress(ctx, true, time.Now())
	ReadProgress(ctx, ReadWaiting, time.Now(), false)
	p := Begin(ctx, "source", "session", "epoch", time.Now())
	base := getAttempt(p).request
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	RequestWritten(p, at(2))
	ReadProgressBytes(ctx, 32, at(10))
	ReadProgress(ctx, ReadBetweenFrames, at(20), true)
	ReadProgress(ctx, ReadDispatch, at(20), false)
	ReadProgress(ctx, ReadBetweenFrames, at(25), false)
	ReadProgress(ctx, ReadWaiting, at(30), false)
	ReadProgressBytes(ctx, 48, at(60))
	ReadProgress(ctx, ReadBetweenFrames, at(70), true)
	ReadProgress(ctx, ReadDispatch, at(70), false)
	stale := Matching(WithControlRead(ctx, at(70)), "source", "old", "epoch")
	recordGrantRead(stale, at(75))
	if getAttempt(p).summary.GrantProgress != nil {
		t.Fatal("stale grant finished the current observation")
	}
	recordGrantRead(Matching(WithControlRead(ctx, at(70)), "source", "session", "epoch"), at(80))
	want := GrantProgressProfile{Schema: 1, Micros: [3]uint32{60000, 15000, 5000}, ElapsedMicros: 80000,
		ByteReads: 2, Bytes: 80, Frames: 2, MaxByteGapMicros: 50000, ByteProgressMeasured: true, Complete: true}
	if got := getAttempt(p).summary.GrantProgress; got == nil || *got != want || !got.Valid("source") {
		t.Fatalf("phase partition or byte gaps lost: %+v", got)
	}
	ReadProgressBytes(ctx, 100, at(90))
	ReadProgress(ctx, ReadBetweenFrames, at(90), true)
	Finish(p, "ok")
	if len(*out) != 1 || !(*out)[0].Valid() || *(*out)[0].GrantProgress != want {
		t.Fatal("grant observation changed after completion")
	}
}

func TestGrantProgressMissingByteObserverAndCancellationDoNotInventEvidence(t *testing.T) {
	_, ctx, out := collected(t)
	StartGrantProgress(ctx, false, time.Now())
	p := Begin(ctx, "source", "s", "e", time.Now())
	base := getAttempt(p).request
	ReadProgressBytes(ctx, 10, base.Add(time.Microsecond))
	ReadProgress(ctx, ReadWaiting, base.Add(2*time.Microsecond), false)
	Finish(p, "error")
	got := (*out)[0].GrantProgress
	if got == nil || got.Complete || got.ByteProgressMeasured || got.Bytes != 0 || got.MaxByteGapMicros != 0 || !got.Valid("source") {
		t.Fatal("cancellation or missing byte observer fabricated a success", got)
	}
	copy := *got
	ReadProgressBytes(ctx, 10, time.Now())
	if *got != copy {
		t.Fatal("retired profile was mutated")
	}
	plain := context.Background()
	StartGrantProgress(plain, true, time.Now())
	ReadProgress(plain, ReadWaiting, time.Now(), false)
	ReadProgressBytes(plain, 1, time.Now())
}

func TestGrantProgressBoundedSaturationAndWindow(t *testing.T) {
	c, ctx, out := collected(t)
	StartGrantProgress(ctx, true, time.Now())
	p := Begin(ctx, "source", "s", "e", time.Now())
	base := getAttempt(p).request
	for i := 0; i < 5; i++ {
		ReadProgressBytes(ctx, 1<<30, base.Add(time.Duration(i)*time.Microsecond))
	}
	ReadProgressBytes(ctx, 1, base.Add(Window+time.Second))
	finishGrantProgress(getSession(ctx), getAttempt(p), base.Add(10*time.Microsecond), false)
	Finish(p, "error")
	got := (*out)[0].GrantProgress
	if got == nil || !got.Saturated || got.Bytes != maxProgressCount || got.ByteReads != 5 || !got.Valid("source") {
		t.Fatal("progress counter overflowed or late sample attached", got)
	}
	p = Begin(ctx, "source", "next", "e", time.Now())
	c.started = time.Now().Add(-Window)
	ReadProgressBytes(ctx, 100, time.Now())
	if getAttempt(p).progress.profile.Bytes != 0 {
		t.Fatal("expired collector accepted progress")
	}
	EndSession(ctx)
}

func TestGrantProgressValidationAndSubMicrosecondConservation(t *testing.T) {
	p := GrantProgressProfile{Schema: 1, ElapsedMicros: 2, Micros: [3]uint32{0, 0, 0}, Complete: true}
	if !p.Valid("source") || p.Valid("receiver") {
		t.Fatal("rounding allowance or source role rejected")
	}
	for _, change := range []func(*GrantProgressProfile){
		func(p *GrantProgressProfile) { p.Schema = 2 },
		func(p *GrantProgressProfile) { p.ElapsedMicros = 3 },
		func(p *GrantProgressProfile) { p.Micros[0] = 3 },
		func(p *GrantProgressProfile) { p.MaxByteGapMicros = 1 },
		func(p *GrantProgressProfile) { p.Bytes = 1 },
		func(p *GrantProgressProfile) { p.Saturated = true },
	} {
		bad := p
		change(&bad)
		if bad.Valid("source") {
			t.Fatal("invalid progress evidence accepted", bad)
		}
	}
	_, ctx, out := collected(t)
	StartGrantProgress(ctx, true, time.Now())
	source := Begin(ctx, "source", "s", "e", time.Now())
	base := getAttempt(source).request
	ReadProgress(ctx, ReadWaiting, base.Add(900*time.Nanosecond), false)
	ReadProgress(ctx, ReadDispatch, base.Add(1800*time.Nanosecond), false)
	finishGrantProgress(getSession(ctx), getAttempt(source), base.Add(2700*time.Nanosecond), true)
	Finish(source, "ok")
	got := (*out)[0].GrantProgress
	if got == nil || !reflect.DeepEqual(got.Micros, [3]uint32{}) || got.ElapsedMicros != 2 || !got.Valid("source") {
		t.Fatal("duration floors violated phase conservation", got)
	}
}

func TestGrantProgressSupersededAndExpiredAttemptsRetainPartialEvidence(t *testing.T) {
	c, ctx, out := collected(t)
	StartGrantProgress(ctx, true, time.Now())
	ReadProgress(ctx, ReadWaiting, time.Now(), false)
	first := Begin(ctx, "source", "first", "epoch", time.Now())
	ReadProgressBytes(ctx, 12, time.Now())
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	if len(*out) != 1 || (*out)[0].GrantProgress == nil || (*out)[0].GrantProgress.Complete || (*out)[0].GrantProgress.Bytes != 12 {
		t.Fatal("superseded attempt lost its partial evidence")
	}
	ReadProgressBytes(ctx, 24, time.Now())
	c.expire()
	if len(*out) != 2 || (*out)[1].GrantProgress == nil || (*out)[1].GrantProgress.Complete || (*out)[1].GrantProgress.Bytes != 24 {
		t.Fatal("expired attempt lost its partial evidence")
	}
	for _, p := range []context.Context{first, next} {
		if !getAttempt(p).summary.GrantProgress.Valid("source") {
			t.Fatal("partial evidence violated conservation")
		}
	}
}
