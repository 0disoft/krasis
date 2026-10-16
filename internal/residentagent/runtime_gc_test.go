package residentagent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productlog"
)

func TestGCWindowsSeparateStoppingAndTotalWithoutInventingMissingCounters(t *testing.T) {
	base := time.Now()
	p := gcMetricSample{at: base.Add(time.Second), known: productlog.GCKnownMask, counters: [5]uint64{100, 200, 3, 400, 500}}
	c := gcMetricSample{at: base.Add(2700 * time.Millisecond), read: 30 * time.Microsecond, known: productlog.GCKnownMask, counters: [5]uint64{800, 50, 4, 600, 900}}
	p.hist[0][0] = 2
	c.hist[0][0] = 2
	c.hist[0][5] = 1
	c.hist[1][4] = 1
	c.hist[2][3] = 2
	w := gcInterval(base, p, c, 1600*time.Millisecond)
	if w.BeginMicros != 1000000 || w.EndMicros != 2700000 || w.GCPauses[5] != 1 || w.GCStopping[4] != 1 || w.Runnable[3] != 2 || w.AssistMicros != 200 || w.GCCPUMicros != 400 || w.Heap != 50 || w.Allocated != 700 || w.Cycles != 1 {
		t.Fatal("GC/stopping/CPU windows mixed", w)
	}
	c.known &^= productlog.GCStoppingKnown
	c.counters[0] = 1
	c.hist[0][0] = 0
	w = gcInterval(base, p, c, 0)
	if w.Known&(productlog.GCStoppingKnown|productlog.GCAllocationKnown|productlog.GCTotalKnown) != 0 || w.GCStopping != ([6]uint64{}) || w.GCPauses != ([6]uint64{}) || w.Allocated != 0 {
		t.Fatal("unknown/reset counter became measured zero", w)
	}
}

func TestGCRingKeepsOrderAndSeparatesPostFailureObservation(t *testing.T) {
	var r gcEvidenceRing
	for i := 0; i < 80; i++ {
		r.add(productlog.GCWindow{BeginMicros: int64(i*100000 + 1), EndMicros: int64((i+1)*100000 + 1)})
	}
	target := gcFailureTrigger{at: time.Now(), notified: 7950000, fault: "fault_source_hook_failed"}
	s := r.snapshot(target, 2)
	if !s.Wrapped || len(s.Samples) != productlog.GCTraceLimit || s.Samples[0].BeginMicros != 1600001 || s.Samples[63].EndMicros != 8000001 || s.SkippedTriggers != 2 || s.Samples[63].EndMicros <= s.TriggerMicros {
		t.Fatal("lost interval order or post-failure boundary", s)
	}
	r.add(productlog.GCWindow{BeginMicros: 8000001, EndMicros: 8100001})
	if s.Samples[0].BeginMicros != 1600001 {
		t.Fatal("old failure trail mutated")
	}
}

func TestGCFailureObserverIsNonblockingAndAllocationFree(t *testing.T) {
	d := newRuntimeDiagnostics()
	d.ObserveFailure(time.Now(), "fault_source_hook_failed")
	if n := testing.AllocsPerRun(100, func() { d.ObserveFailure(time.Now(), "fault_source_hook_failed") }); n != 0 {
		t.Fatal("failure callback allocates", n)
	}
	if d.gcSkipped.Load() == 0 || len(d.gcFailures) != 1 {
		t.Fatal("notification queue grew or dropped silently")
	}
}

func BenchmarkGCRuntimeSampler(b *testing.B) {
	read := newGCSampler()
	read()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		read()
	}
}

type gcTestSink struct {
	trails    []productlog.GCTrail
	summaries int
}

func (s *gcTestSink) Runtime(productlog.RuntimeSummary) { s.summaries++ }
func (s *gcTestSink) GCTrail(t productlog.GCTrail)      { s.trails = append(s.trails, t) }
func (s *gcTestSink) Event(string)                      {}

func TestGCFailureNotificationIsDrainedWhenShutdownWins(t *testing.T) {
	d := newRuntimeDiagnostics()
	d.ObserveFailure(time.Now(), "fault_source_hook_failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sink := &gcTestSink{}
	d.run(ctx, sink)
	if len(sink.trails) != 1 || sink.summaries != 1 || len(d.gcFailures) != 0 {
		t.Fatal("shutdown lost or repeated failure evidence", sink)
	}
}
