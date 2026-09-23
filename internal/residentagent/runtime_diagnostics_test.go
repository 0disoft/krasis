package residentagent

import (
	"github.com/0disoft/krasis/internal/transport"
	"math"
	"runtime/metrics"
	"sync"
	"testing"
	"time"
)

func TestRuntimeDiagnosticsSeparatesReadDispatchAndValidatedHeartbeat(t *testing.T) {
	d := newRuntimeDiagnostics()
	start := d.started.Add(time.Second)
	d.ObserveReadStarted(start)
	d.ObserveReadBytes(7, start.Add(time.Second))
	if s := d.snapshot(start.Add(3 * time.Second)); s.ReadAgeMicros != 3000000 || s.DispatchKind != "idle" || s.HeartbeatReceived {
		t.Fatalf("idle read=%+v", s)
	} else if s.FrameReadBytes != 7 || !s.ReadByteObserved || s.LastReadByteAgeMicros != 2000000 {
		t.Fatalf("partial frame progress=%+v", s)
	}
	d.ObserveReadCompleted(start.Add(3 * time.Second))
	d.ObserveDispatchStarted(transport.MessageInput, start.Add(3*time.Second))
	if s := d.snapshot(start.Add(5 * time.Second)); s.ReadAgeMicros != 0 || s.DispatchAgeMicros != 2000000 || s.DispatchKind != "input" || s.FramesReceived != 1 || s.HeartbeatReceived {
		t.Fatalf("stalled dispatch=%+v", s)
	}
	d.ObserveDispatchCompleted(start.Add(6 * time.Second))
	d.ObserveHeartbeatReceived("not-exported", start.Add(6*time.Second))
	d.ObserveTimerLateness(600 * time.Millisecond)
	s := d.snapshot(start.Add(7 * time.Second))
	if s.DispatchKind != "idle" || s.DispatchMaxMicros != 3000000 || s.Dispatches != 1 || s.HeartbeatReceiveAgeMicros != 1000000 || !s.HeartbeatReceived || s.TimerLateMicros != 600000 {
		t.Fatalf("completed=%+v", s)
	}
	if next := d.snapshot(start.Add(8 * time.Second)); next.Dispatches != 0 || next.DispatchMaxMicros != 0 || next.TimerLateMicros != 0 {
		t.Fatalf("interval counters repeated=%+v", next)
	}
	d.ObserveReadConnectionStarted()
	if reset := d.snapshot(start.Add(9 * time.Second)); reset.ReadByteObserved || reset.LastReadByteAgeMicros != 0 || reset.FrameReadBytes != 0 {
		t.Fatalf("new connection inherited old read progress=%+v", reset)
	}
	d.ObserveDispatchStarted(transport.MessageInput, start.Add(10*time.Second))
	d.ObserveReadConnectionStarted()
	if reset := d.snapshot(start.Add(11 * time.Second)); reset.DispatchKind != "idle" || reset.DispatchAgeMicros != 0 {
		t.Fatalf("new connection inherited old dispatch=%+v", reset)
	}
}

func TestRuntimeDiagnosticsReportsBoundedQueueAggregates(t *testing.T) {
	d := newRuntimeDiagnostics()
	d.ObserveSourceQueueEnqueued(1)
	d.ObserveSourceQueueEnqueued(3)
	d.ObserveSourceQueueDequeued(250 * time.Millisecond)
	d.ObserveSourceQueueCoalesced()
	d.ObserveSourceQueueDiscardedStale()
	s := d.snapshot(d.started.Add(time.Second))
	if s.QueueHighWatermark != 3 || s.QueueEnqueued != 2 || s.QueueDequeued != 1 || s.QueueCoalesced != 1 || s.QueueStaleDiscards != 1 || s.QueueMaxResidenceMicros != 250000 {
		t.Fatalf("queue aggregates=%+v", s)
	}
	next := d.snapshot(d.started.Add(2 * time.Second))
	if next.QueueHighWatermark != 0 || next.QueueEnqueued != 0 || next.QueueDequeued != 0 || next.QueueCoalesced != 0 || next.QueueStaleDiscards != 0 || next.QueueMaxResidenceMicros != 0 {
		t.Fatalf("queue counters repeated=%+v", next)
	}
}

func TestRuntimeSamplingUsesHistogramDeltasAndKnownOSCounters(t *testing.T) {
	h := &metrics.Float64Histogram{Buckets: []float64{0, .00005, .0005, .005, .05, .5, math.Inf(1)}, Counts: []uint64{1, 2, 3, 4, 5, 6}}
	if got := aggregateHistogram(h); got != [6]uint64{1, 2, 3, 4, 5, 6} {
		t.Fatal(got)
	}
	d := newRuntimeDiagnostics()
	previous := runtimeSample{at: d.started, allocated: 100, cpu: 500, cpuKnown: true, gc: [6]uint64{4}}
	current := runtimeSample{at: d.started.Add(5 * time.Second), allocated: 600, heap: 50, cpu: 800, cpuKnown: true, workingSetKnown: true, workingSet: 900, gc: [6]uint64{7}, scheduler: [6]uint64{0, 3}}
	s := d.sample(current, previous)
	if s.IntervalMicros != 5000000 || s.AllocatedBytes != 500 || s.CPUMicros != 300 || s.GCPauses[0] != 3 || s.RunnableDelays[1] != 3 || s.WorkingSetBytes != 900 || !s.WorkingSetKnown {
		t.Fatalf("sample=%+v", s)
	}
	current.cpuKnown = false
	current.cpu = 0
	current.allocated = 50
	s = d.sample(current, previous)
	if s.CPUKnown || s.CPUMicros != 0 || s.AllocatedBytes != 0 {
		t.Fatalf("unknown/reset counters fabricated=%+v", s)
	}
	read := newRuntimeSampler()
	actual := read()
	if actual.heap == 0 || actual.allocated == 0 || actual.at.IsZero() {
		t.Fatalf("runtime metrics unavailable=%+v", actual)
	}
}

func TestRuntimeObserverIsAllocationFreeAndRaceSafe(t *testing.T) {
	d := newRuntimeDiagnostics()
	now := time.Now()
	if n := testing.AllocsPerRun(100, func() {
		d.ObserveReadStarted(now)
		d.ObserveReadCompleted(now)
		d.ObserveDispatchStarted(transport.MessageInput, now)
		d.ObserveDispatchCompleted(now)
	}); n != 0 {
		t.Fatalf("callback allocations=%v", n)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				d.ObserveHeartbeatSent("", time.Now())
				d.ObserveTimerLateness(time.Millisecond)
				d.snapshot(time.Now())
			}
		}()
	}
	wg.Wait()
}

func TestHookTimingDistinguishesIdleFromRawProgressDuringCallback(t *testing.T) {
	d := newRuntimeDiagnostics()
	start := d.started.Add(time.Second)
	if s := d.snapshot(start); s.HookObserved || s.RawInputs != 0 || s.HookCalls != 0 {
		t.Fatal("idle fabricated progress")
	}
	d.ObserveHookStarted(start)
	d.ObserveRawInput(start.Add(time.Second))
	s := d.snapshot(start.Add(time.Second))
	if s.HookActiveMicros != 1000000 || s.RawInputs != 1 || s.HookCalls != 0 {
		t.Fatalf("stalled callback=%+v", s)
	}
	d.ObserveHookCompleted(start.Add(2 * time.Second))
	s = d.snapshot(start.Add(3 * time.Second))
	if s.HookCalls != 1 || s.HookMaxMicros != 2000000 || s.LastHookAgeMicros != 1000000 || !s.HookObserved || s.HookActiveMicros != 0 {
		t.Fatalf("completed callback=%+v", s)
	}
}
