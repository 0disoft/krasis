package residentagent

import (
	"math"
	"runtime/metrics"
	"time"

	"github.com/0disoft/krasis/internal/productlog"
)

const gcSampleInterval = 100 * time.Millisecond
const gcTrailBudget = 8

type gcFailureTrigger struct {
	at        time.Time
	notified  int64
	fault     string
	heartbeat productlog.HeartbeatProgress
}

// Observer callbacks enqueue bounded scalar evidence. No runtime/OS query,
// JSON encoding, disk operation or waiting is performed on the failed path.
func (d *runtimeDiagnostics) ObserveFailure(at time.Time, fault string) {
	heartbeat := d.heartbeatSnapshot()
	notified := d.stamp(time.Now())
	heartbeat.AtMicros = notified
	select {
	case d.gcFailures <- gcFailureTrigger{at: at, notified: notified, fault: fault, heartbeat: heartbeat}:
	default:
		d.gcSkipped.Add(1)
	}
}

type gcMetricSample struct {
	at       time.Time
	read     time.Duration
	known    uint16
	hist     [3][6]uint64
	counters [5]uint64 // allocated bytes, heap bytes, cycles, assist us, GC CPU us
}

func newGCSampler() func() gcMetricSample {
	samples := []metrics.Sample{
		{Name: "/sched/pauses/total/gc:seconds"},
		{Name: "/sched/pauses/stopping/gc:seconds"},
		{Name: "/sched/latencies:seconds"},
		{Name: "/gc/heap/allocs:bytes"},
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/gc/cycles/total:gc-cycles"},
		{Name: "/cpu/classes/gc/mark/assist:cpu-seconds"},
		{Name: "/cpu/classes/gc/total:cpu-seconds"},
	}
	return func() gcMetricSample {
		started := time.Now()
		metrics.Read(samples)
		s := gcMetricSample{}
		for i := 0; i < 3; i++ {
			if samples[i].Value.Kind() == metrics.KindFloat64Histogram {
				s.known |= 1 << i
				s.hist[i] = aggregateHistogram(samples[i].Value.Float64Histogram())
			}
		}
		for i := 3; i < 8; i++ {
			v := samples[i].Value
			if i < 6 && v.Kind() == metrics.KindUint64 {
				s.known |= 1 << i
				s.counters[i-3] = v.Uint64()
			} else if i >= 6 && v.Kind() == metrics.KindFloat64 {
				x := v.Float64() * 1e6
				if !math.IsNaN(x) && !math.IsInf(x, 0) && x >= 0 && x < float64(^uint64(0)) {
					s.known |= 1 << i
					s.counters[i-3] = uint64(x)
				}
			}
		}
		s.at = time.Now()
		s.read = s.at.Sub(started)
		return s
	}
}

// Counts in the total and stopping histograms are independent observations.
// Do not subtract their bucket maxima or call the difference GC work duration.
func gcInterval(base time.Time, previous, current gcMetricSample, late time.Duration) productlog.GCWindow {
	w := productlog.GCWindow{BeginMicros: max(1, previous.at.Sub(base).Microseconds()), EndMicros: max(1, current.at.Sub(base).Microseconds()), ReadMicros: max(0, current.read.Microseconds()), LateMicros: max(0, late.Microseconds()), Known: previous.known & current.known}
	hist := [3]*[6]uint64{&w.GCPauses, &w.GCStopping, &w.Runnable}
	for i, target := range hist {
		bit := uint16(1 << i)
		if w.Known&bit == 0 {
			continue
		}
		for j, n := range current.hist[i] {
			if n < previous.hist[i][j] {
				w.Known &^= bit
				*target = [6]uint64{}
				break
			}
			target[j] = n - previous.hist[i][j]
		}
	}
	values := [5]*uint64{&w.Allocated, &w.Heap, &w.Cycles, &w.AssistMicros, &w.GCCPUMicros}
	for i, target := range values {
		bit := uint16(1 << (i + 3))
		if i == 1 { // Heap is a current gauge, not a counter delta.
			if current.known&bit != 0 {
				w.Known |= bit
				*target = current.counters[i]
			}
			continue
		}
		if w.Known&bit == 0 {
			continue
		}
		if current.counters[i] < previous.counters[i] {
			w.Known &^= bit
			continue
		}
		*target = current.counters[i] - previous.counters[i]
	}
	return w
}

type gcEvidenceRing struct {
	windows     [productlog.GCTraceLimit]productlog.GCWindow
	next, count int
	wrapped     bool
}

func (r *gcEvidenceRing) add(w productlog.GCWindow) {
	if w.EndMicros <= w.BeginMicros {
		return
	}
	r.windows[r.next] = w
	r.next = (r.next + 1) % len(r.windows)
	if r.count < len(r.windows) {
		r.count++
	} else {
		r.wrapped = true
	}
}

func (r *gcEvidenceRing) snapshot(t gcFailureTrigger, skipped uint64) productlog.GCTrail {
	s := productlog.GCTrail{Schema: 1, TriggerMicros: t.notified, TriggerObservedAt: t.at.Format(time.RFC3339Nano), Fault: t.fault, Wrapped: r.wrapped, SkippedTriggers: skipped, Samples: make([]productlog.GCWindow, r.count)}
	if t.heartbeat.Schema != 0 {
		s.Heartbeat = &t.heartbeat
	}
	start := (r.next - r.count + len(r.windows)) % len(r.windows)
	for i := range s.Samples {
		s.Samples[i] = r.windows[(start+i)%len(r.windows)]
	}
	return s
}
