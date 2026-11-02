package residentagent

import (
	"time"

	"github.com/0disoft/krasis/internal/productlog"
)

const (
	gcObservationThreshold = 100 * time.Millisecond
	gcObservationCooldown  = 30 * time.Second
	gcObservationBudget    = 8
)

// All checks run on the existing sampler, never on input or heartbeat paths.
// Histogram triggers name aggregate buckets, not exact pause durations.
func gcObservationReason(w productlog.GCWindow) string {
	if w.LateMicros >= gcObservationThreshold.Microseconds() {
		return "timer_late"
	}
	if w.ReadMicros >= gcObservationThreshold.Microseconds() {
		return "metrics_read_slow"
	}
	if w.OS.ReadMicros >= uint64(gcObservationThreshold.Microseconds()) {
		return "os_read_slow"
	}
	for _, h := range []struct {
		known  uint16
		counts [6]uint64
		reason string
	}{
		{productlog.GCStoppingKnown, w.GCStopping, "gc_stopping_bucket"},
		{productlog.GCTotalKnown, w.GCPauses, "gc_pause_bucket"},
		{productlog.GCSchedulerKnown, w.Runnable, "runnable_bucket"},
	} {
		if w.Known&h.known != 0 && (h.counts[4] != 0 || h.counts[5] != 0) {
			return h.reason
		}
	}
	return ""
}

type gcObservationLimit struct {
	last    time.Time
	count   int
	skipped uint64
}

func (l *gcObservationLimit) admit(at time.Time) bool {
	if l.count >= gcObservationBudget || !l.last.IsZero() && at.Sub(l.last) < gcObservationCooldown {
		l.skipped++
		return false
	}
	l.count++
	l.last = at
	return true
}
