package handoff

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestSocketQueueSnapshotKeepsGrantEndpointAndNoNetworkInference(t *testing.T) {
	base := time.Now()
	at := func(us int) time.Time { return base.Add(time.Duration(us) * time.Microsecond) }
	p := &grantProgress{start: base, socketCount: 6}
	p.socketSamples[0] = socketSample{started: at(9999), finished: at(10000), ok: true}
	p.socketSamples[1] = socketSample{started: at(19998), finished: at(20000), bytes: 116, ok: true}
	p.socketSamples[2] = socketSample{started: at(29998), finished: at(30000), bytes: 232, ok: true}
	p.socketSamples[3] = socketSample{started: at(39999), finished: at(40000)}
	p.socketSamples[4] = socketSample{started: at(40000), finished: at(40001), bytes: 999, ok: true}
	p.socketSamples[5] = socketSample{started: at(-1), finished: at(1), bytes: 999, ok: true}
	got := p.socketSnapshot(at(40000))
	want := SocketQueueProfile{Samples: 3, PositiveSamples: 2, Failures: 1, FirstQueuedMicros: 20000,
		LastEmptyMicros: 10000, MaxQueuedBytes: 232, MaxSampleGapMicros: 10000, MaxQueryMicros: 2}
	if got != want || !got.valid(40000) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	if got.valid(19999) {
		t.Fatal("queue evidence outside grant span accepted")
	}
	if got := (&grantProgress{start: base}).socketSnapshot(at(40000)); got != (SocketQueueProfile{}) {
		t.Fatal("missing probe became empty queue evidence", got)
	}
	p.socketCount = 1
	p.socketSamples[0] = socketSample{started: at(1), finished: at(0), ok: true}
	if got := p.socketSnapshot(at(10)); got != (SocketQueueProfile{}) {
		t.Fatal("negative query duration", got)
	}
}

func TestSocketProbeIsIdleBoundedJoinedAndFreezesGrant(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, ctx, _ := collected(t)
		calls := 0
		stop := ConfigureSocketReceiveProbe(ctx, func() (uint32, bool) { calls++; return 116, true })
		defer stop()
		StartGrantProgress(ctx, true, time.Now())
		time.Sleep(time.Second)
		if calls != 0 {
			t.Fatal("idle socket polled")
		}
		p := Begin(ctx, "source", "session", "epoch", time.Now())
		time.Sleep(time.Second)
		synctest.Wait()
		if calls != socketProbeLimit {
			t.Fatal("unbounded or missing polling", calls)
		}
		Granted(WithControlRead(p, time.Now()))
		got := getAttempt(p).summary.GrantProgress
		if got == nil || got.SocketQueue.Samples != socketProbeLimit || !got.Valid("source") {
			t.Fatalf("missing bounded profile %+v", got)
		}
		frozen := *got
		stop() // Repeated shutdown is safe and joins the observer.
		time.Sleep(time.Second)
		if calls != socketProbeLimit || *got != frozen {
			t.Fatal("observer outlived stop/grant")
		}
	})
}

func TestSocketProbeUnavailableStopsAndPendingQueryCannotCrossEpoch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, ctx, _ := collected(t)
		calls := 0
		stop := ConfigureSocketReceiveProbe(ctx, func() (uint32, bool) { calls++; return 0, false })
		defer stop()
		StartGrantProgress(ctx, true, time.Now())
		p := Begin(ctx, "source", "session", "epoch", time.Now())
		time.Sleep(time.Second)
		synctest.Wait()
		Granted(WithControlRead(p, time.Now()))
		got := getAttempt(p).summary.GrantProgress.SocketQueue
		if calls != 1 || got.Samples != 0 || got.Failures != 1 {
			t.Fatal("unavailable became zero/network evidence", calls, got)
		}
	})
	synctest.Test(t, func(t *testing.T) {
		_, ctx, _ := collected(t)
		StartGrantProgress(ctx, true, time.Now())
		p := Begin(ctx, "source", "session", "old", time.Now())
		s := getSession(ctx)
		active, supported := sampleSocketQueue(s, func() (uint32, bool) {
			// Query is outside the collector lock. Retire and replace while it runs.
			Finish(p, "error")
			Begin(ctx, "source", "session", "new", time.Now())
			return 116, true
		})
		if active || !supported || s.source.progress.socketCount != 0 {
			t.Fatal("stale query attached to a new epoch")
		}
	})
}

func TestSocketProbeCancellationJoinsWithoutAnotherRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, parent, _ := collected(t)
		ctx, cancel := context.WithCancel(parent)
		calls := 0
		stop := ConfigureSocketReceiveProbe(ctx, func() (uint32, bool) { calls++; return 0, true })
		StartGrantProgress(ctx, true, time.Now())
		Begin(ctx, "source", "session", "epoch", time.Now())
		cancel()
		stop()
		time.Sleep(time.Second)
		if calls != 0 {
			t.Fatal("canceled probe queried socket")
		}
	})
}

func TestSocketProbeStopWaitsForItsOwnedQuery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, ctx, _ := collected(t)
		entered, release := make(chan struct{}), make(chan struct{})
		stop := ConfigureSocketReceiveProbe(ctx, func() (uint32, bool) {
			close(entered)
			<-release
			return 0, true
		})
		StartGrantProgress(ctx, true, time.Now())
		Begin(ctx, "source", "session", "epoch", time.Now())
		<-entered
		joined := make(chan struct{})
		go func() { stop(); close(joined) }()
		synctest.Wait()
		select {
		case <-joined:
			t.Fatal("query detached on cancellation")
		default:
		}
		close(release)
		<-joined
	})
}
