package residentagent

import (
	"context"
	"runtime/metrics"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

// Session callbacks only update atomics. Runtime/OS sampling and JSON/disk work
// stay off input, reader and heartbeat paths. Counters are diagnostic hints, not
// lease authority; snapshots across atomics are deliberately approximate.
type runtimeDiagnostics struct {
	hookStart, hookLast, hookMax                            atomic.Int64
	hookCalls, rawInputs                                    atomic.Uint64
	started                                                 time.Time
	readStart, lastReadByte, dispatchStart, send, receive   atomic.Int64
	kind                                                    atomic.Uint32
	frames, frameReadBytes, dispatches                      atomic.Uint64
	dispatchMax, timerMax                                   atomic.Int64
	injectionStart, injectionMax                            atomic.Int64
	injections                                              atomic.Uint64
	queueHigh, queueEnqueued, queueDequeued, queueCoalesced atomic.Uint64
	queueStale                                              atomic.Uint64
	queueResidenceMax                                       atomic.Int64
	gcFailures                                              chan gcFailureTrigger
	gcSkipped                                               atomic.Uint64
	dispatchRead, coordinatorQueued, coordinatorAcquired    atomic.Int64
	dispatchInjected, dispatchInjectionMax                  atomic.Int64
	dispatchInjections                                      atomic.Uint64
	slowDispatch                                            chan productlog.DispatchSpan
	slowDispatchDropped                                     atomic.Uint64
}

func (d *runtimeDiagnostics) ObserveHookStarted(t time.Time) { d.hookStart.Store(d.stamp(t)) }
func (d *runtimeDiagnostics) ObserveHookCompleted(t time.Time) {
	stamp := d.stamp(t)
	start := d.hookStart.Swap(0)
	if start > 0 {
		observeMaximum(&d.hookMax, max(0, stamp-start))
	}
	d.hookLast.Store(stamp)
	d.hookCalls.Add(1)
}
func (d *runtimeDiagnostics) ObserveRawInput(time.Time) { d.rawInputs.Add(1) }

func newRuntimeDiagnostics() *runtimeDiagnostics {
	return &runtimeDiagnostics{started: time.Now(), gcFailures: make(chan gcFailureTrigger, 1), slowDispatch: make(chan productlog.DispatchSpan, 32)}
}
func (d *runtimeDiagnostics) stamp(t time.Time) int64 { return max(1, t.Sub(d.started).Microseconds()) }
func (d *runtimeDiagnostics) ObserveReadConnectionStarted() {
	d.readStart.Store(0)
	d.lastReadByte.Store(0)
	d.dispatchStart.Store(0)
	d.injectionStart.Store(0)
	d.kind.Store(0)
	d.dispatchRead.Store(0)
}
func (d *runtimeDiagnostics) ObserveReadBytes(count int, at time.Time) {
	if count <= 0 {
		return
	}
	d.frameReadBytes.Add(uint64(count))
	d.lastReadByte.Store(d.stamp(at))
}
func (d *runtimeDiagnostics) ObserveReadStarted(t time.Time) { d.readStart.Store(d.stamp(t)) }
func (d *runtimeDiagnostics) ObserveReadCompleted(t time.Time) {
	d.readStart.Store(0)
	d.dispatchRead.Store(d.stamp(t))
	d.frames.Add(1)
}
func (d *runtimeDiagnostics) ObserveDispatchStarted(k transport.MessageKind, t time.Time) {
	d.coordinatorQueued.Store(0)
	d.coordinatorAcquired.Store(0)
	d.dispatchInjected.Store(0)
	d.dispatchInjectionMax.Store(0)
	d.dispatchInjections.Store(0)
	d.kind.Store(uint32(k))
	d.dispatchStart.Store(d.stamp(t))
}
func (d *runtimeDiagnostics) ObserveDispatchCompleted(t time.Time) {
	start := d.dispatchStart.Swap(0)
	if start > 0 {
		observeMaximum(&d.dispatchMax, max(0, d.stamp(t)-start))
		d.observeSlowDispatch(start, d.stamp(t))
	}
	d.dispatches.Add(1)
}
func (d *runtimeDiagnostics) ObserveHeartbeatSent(_ string, t time.Time) { d.send.Store(d.stamp(t)) }
func (d *runtimeDiagnostics) ObserveHeartbeatReceived(_ string, t time.Time) {
	d.receive.Store(d.stamp(t))
}
func (d *runtimeDiagnostics) ObserveTimerLateness(delay time.Duration) {
	observeMaximum(&d.timerMax, max(0, delay.Microseconds()))
}
func (*runtimeDiagnostics) ObserveDestinationInputReceived()                                {}
func (*runtimeDiagnostics) ObserveDestinationInputDisposition(application.InputDisposition) {}
func (d *runtimeDiagnostics) ObserveDestinationInjectionStarted(t time.Time) {
	d.injectionStart.Store(d.stamp(t))
}
func (d *runtimeDiagnostics) ObserveDestinationInjectionCompleted(t time.Time) {
	start := d.injectionStart.Swap(0)
	if start > 0 {
		elapsed := max(0, d.stamp(t)-start)
		observeMaximum(&d.injectionMax, elapsed)
		d.injections.Add(1)
		if dispatch := d.dispatchStart.Load(); dispatch > 0 && start >= dispatch {
			d.dispatchInjected.Add(elapsed)
			observeMaximum(&d.dispatchInjectionMax, elapsed)
			d.dispatchInjections.Add(1)
		}
	}
}
func (d *runtimeDiagnostics) ObserveSourceQueueEnqueued(depth uint64) {
	d.queueEnqueued.Add(1)
	for old := d.queueHigh.Load(); depth > old; old = d.queueHigh.Load() {
		if d.queueHigh.CompareAndSwap(old, depth) {
			break
		}
	}
}
func (d *runtimeDiagnostics) ObserveSourceQueueDequeued(residence time.Duration) {
	d.queueDequeued.Add(1)
	observeMaximum(&d.queueResidenceMax, max(0, residence.Microseconds()))
}
func (d *runtimeDiagnostics) ObserveSourceQueueCoalesced()      { d.queueCoalesced.Add(1) }
func (d *runtimeDiagnostics) ObserveSourceQueueDiscardedStale() { d.queueStale.Add(1) }
func observeMaximum(target *atomic.Int64, value int64) {
	for old := target.Load(); value > old; old = target.Load() {
		if target.CompareAndSwap(old, value) {
			return
		}
	}
}
func (d *runtimeDiagnostics) snapshot(now time.Time) productlog.RuntimeSummary {
	stamp := d.stamp(now)
	age := func(start int64) int64 {
		if start == 0 {
			return 0
		}
		return max(0, stamp-start)
	}
	s := productlog.RuntimeSummary{DispatchKind: "idle", ReadAgeMicros: age(d.readStart.Load()), FrameReadBytes: d.frameReadBytes.Swap(0), FramesReceived: d.frames.Swap(0), Dispatches: d.dispatches.Swap(0), DispatchMaxMicros: d.dispatchMax.Swap(0), TimerLateMicros: d.timerMax.Swap(0)}
	s.InjectionAgeMicros = age(d.injectionStart.Load())
	s.InjectionMaxMicros = d.injectionMax.Swap(0)
	s.Injections = d.injections.Swap(0)
	lastReadByte := d.lastReadByte.Load()
	s.ReadByteObserved = lastReadByte > 0
	s.LastReadByteAgeMicros = age(lastReadByte)
	if start := d.dispatchStart.Load(); start != 0 {
		s.DispatchAgeMicros = age(start)
		switch transport.MessageKind(d.kind.Load()) {
		case transport.MessageControl:
			s.DispatchKind = "control"
		case transport.MessageInput:
			s.DispatchKind = "input"
		case transport.MessageHeartbeat:
			s.DispatchKind = "heartbeat"
		case transport.MessageDisconnect:
			s.DispatchKind = "disconnect"
		default:
			s.DispatchKind = "unknown"
		}
	}
	send, receive := d.send.Load(), d.receive.Load()
	s.HookCalls = d.hookCalls.Swap(0)
	s.RawInputs = d.rawInputs.Swap(0)
	s.QueueHighWatermark = d.queueHigh.Swap(0)
	s.QueueEnqueued = d.queueEnqueued.Swap(0)
	s.QueueDequeued = d.queueDequeued.Swap(0)
	s.QueueCoalesced = d.queueCoalesced.Swap(0)
	s.QueueStaleDiscards = d.queueStale.Swap(0)
	s.QueueMaxResidenceMicros = d.queueResidenceMax.Swap(0)
	s.HookMaxMicros = d.hookMax.Swap(0)
	s.HookActiveMicros = age(d.hookStart.Load())
	lastHook := d.hookLast.Load()
	s.LastHookAgeMicros = age(lastHook)
	s.HookObserved = lastHook > 0
	s.HeartbeatSent = send > 0
	s.HeartbeatReceived = receive > 0
	s.HeartbeatSendAgeMicros = age(send)
	s.HeartbeatReceiveAgeMicros = age(receive)
	d.drainSlowDispatch(&s)
	return s
}

type runtimeSample struct {
	at                               time.Time
	heap, allocated, cpu, workingSet uint64
	cpuKnown, workingSetKnown        bool
	gc, scheduler                    [6]uint64
}

// runtime/metrics avoids the stop-the-world ReadMemStats API. Histogram storage
// is reused by this one sampler, and only fixed-size aggregate values escape it.
func newRuntimeSampler() func() runtimeSample {
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/heap/allocs:bytes"}, {Name: "/sched/pauses/total/gc:seconds"}, {Name: "/sched/latencies:seconds"}}
	return func() runtimeSample {
		metrics.Read(samples)
		s := runtimeSample{at: time.Now()}
		if samples[0].Value.Kind() == metrics.KindUint64 {
			s.heap = samples[0].Value.Uint64()
		}
		if samples[1].Value.Kind() == metrics.KindUint64 {
			s.allocated = samples[1].Value.Uint64()
		}
		if samples[2].Value.Kind() == metrics.KindFloat64Histogram {
			s.gc = aggregateHistogram(samples[2].Value.Float64Histogram())
		}
		if samples[3].Value.Kind() == metrics.KindFloat64Histogram {
			s.scheduler = aggregateHistogram(samples[3].Value.Float64Histogram())
		}
		s.workingSet, s.workingSetKnown, s.cpu, s.cpuKnown = processRuntimeCounters()
		return s
	}
}
func aggregateHistogram(h *metrics.Float64Histogram) (result [6]uint64) {
	limits := [5]float64{.0001, .001, .01, .1, 1}
	for i, count := range h.Counts {
		bucket := 5
		for j, limit := range limits {
			if h.Buckets[i+1] <= limit {
				bucket = j
				break
			}
		}
		result[bucket] += count
	}
	return
}
func counterDelta(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}
func (d *runtimeDiagnostics) sample(current, previous runtimeSample) productlog.RuntimeSummary {
	s := d.snapshot(current.at)
	s.IntervalMicros = max(1, current.at.Sub(previous.at).Microseconds())
	s.HeapBytes = current.heap
	s.AllocatedBytes = counterDelta(current.allocated, previous.allocated)
	s.WorkingSetBytes = current.workingSet
	s.WorkingSetKnown = current.workingSetKnown
	s.CPUKnown = current.cpuKnown && previous.cpuKnown
	if s.CPUKnown {
		s.CPUMicros = counterDelta(current.cpu, previous.cpu)
	}
	for i := range s.GCPauses {
		s.GCPauses[i] = counterDelta(current.gc[i], previous.gc[i])
		s.RunnableDelays[i] = counterDelta(current.scheduler[i], previous.scheduler[i])
	}
	return s
}

type runtimeDiagnosticSink interface {
	Runtime(productlog.RuntimeSummary)
	GCTrail(productlog.GCTrail)
	Event(string)
}

func (d *runtimeDiagnostics) run(ctx context.Context, log runtimeDiagnosticSink) {
	read := newRuntimeSampler()
	previous := read()
	readGC := newGCSampler()
	previousGC := readGC()
	var ring gcEvidenceRing
	trails := 0
	sampleGC := func(late time.Duration) {
		current := readGC()
		ring.add(gcInterval(d.started, previousGC, current, late))
		previousGC = current
	}
	flushFailure := func(t gcFailureTrigger) {
		if trails >= gcTrailBudget {
			d.gcSkipped.Add(1)
			return
		}
		sampleGC(0)
		log.GCTrail(ring.snapshot(t, d.gcSkipped.Load()))
		trails++
	}
	gcTicker := time.NewTicker(gcSampleInterval)
	defer gcTicker.Stop()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// The owning runner joins its callbacks before explicit stopProgress.
			// Drain the failure notification even if shutdown won this select.
			select {
			case t := <-d.gcFailures:
				flushFailure(t)
			default:
			}
			if d.gcSkipped.Load() > 0 {
				log.Event("runtime_gc_trail_limited")
			}
			log.Runtime(d.sample(read(), previous))
			return
		case t := <-d.gcFailures:
			flushFailure(t)
		case scheduled := <-gcTicker.C:
			sampleGC(time.Since(scheduled))
		case scheduled := <-ticker.C:
			d.ObserveTimerLateness(time.Since(scheduled))
			current := read()
			log.Runtime(d.sample(current, previous))
			previous = current
		}
	}
}
