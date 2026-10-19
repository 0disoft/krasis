package productlog

import (
	"strings"
	"time"
)

const GCTraceLimit = 64

// Each bit declares the availability of a metric for this interval. Missing or
// reset counters are unknown, not a measured zero. CPU values are runtime
// estimates, not OS CPU time. Histogram buckets use the RuntimeSummary bounds.
const (
	GCTotalKnown uint16 = 1 << iota
	GCStoppingKnown
	GCSchedulerKnown
	GCAllocationKnown
	GCHeapKnown
	GCCyclesKnown
	GCAssistKnown
	GCCPUKnown
	GCKnownMask = (1 << iota) - 1
)

type GCWindow struct {
	BeginMicros  int64     `json:"beginMicros"`
	EndMicros    int64     `json:"endMicros"`
	ReadMicros   int64     `json:"readMicros"`
	LateMicros   int64     `json:"lateMicros"`
	Known        uint16    `json:"known"`
	GCPauses     [6]uint64 `json:"gcPauses"`
	GCStopping   [6]uint64 `json:"gcStopping"`
	Runnable     [6]uint64 `json:"runnable"`
	Allocated    uint64    `json:"allocatedBytes"`
	Heap         uint64    `json:"heapBytes"`
	Cycles       uint64    `json:"cycles"`
	AssistMicros uint64    `json:"assistMicros"`
	GCCPUMicros  uint64    `json:"gcCPUMicros"`
}

// This is a bounded observation trail, never exact GC start/end timestamps.
// TriggerMicros is the monotonic callback-notification time in the agent run;
// TriggerObservedAt is the original failure observation's wall time. A sample
// ending after the trigger cannot be labelled as wholly preceding the failure.
type GCTrail struct {
	Schema            uint8              `json:"schema"`
	TriggerMicros     int64              `json:"triggerMicros"`
	TriggerObservedAt string             `json:"triggerObservedAt"`
	Fault             string             `json:"fault"`
	Wrapped           bool               `json:"wrapped"`
	SkippedTriggers   uint64             `json:"skippedTriggers"`
	Samples           []GCWindow         `json:"samples"`
	Heartbeat         *HeartbeatProgress `json:"heartbeat,omitempty"`
}

func (s GCTrail) valid() bool {
	if s.Heartbeat != nil && (!s.Heartbeat.valid() || s.Heartbeat.AtMicros != s.TriggerMicros) {
		return false
	}
	if s.Schema != 1 || s.TriggerMicros < 1 || !strings.HasPrefix(s.Fault, "fault_") || !allowed[s.Fault] || len(s.Samples) == 0 || len(s.Samples) > GCTraceLimit {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, s.TriggerObservedAt); err != nil || len(s.TriggerObservedAt) > 64 {
		return false
	}
	previous := int64(0)
	for _, w := range s.Samples {
		if w.BeginMicros < 1 || w.EndMicros <= w.BeginMicros || w.BeginMicros < previous || w.ReadMicros < 0 || w.ReadMicros > w.EndMicros-w.BeginMicros || w.LateMicros < 0 || w.Known & ^uint16(GCKnownMask) != 0 {
			return false
		}
		if w.Known&GCTotalKnown == 0 && w.GCPauses != ([6]uint64{}) || w.Known&GCStoppingKnown == 0 && w.GCStopping != ([6]uint64{}) || w.Known&GCSchedulerKnown == 0 && w.Runnable != ([6]uint64{}) {
			return false
		}
		for _, m := range []struct {
			bit   uint16
			value uint64
		}{{GCAllocationKnown, w.Allocated}, {GCHeapKnown, w.Heap}, {GCCyclesKnown, w.Cycles}, {GCAssistKnown, w.AssistMicros}, {GCCPUKnown, w.GCCPUMicros}} {
			if w.Known&m.bit == 0 && m.value != 0 {
				return false
			}
		}
		previous = w.EndMicros
	}
	return true
}

func init() { allowed["runtime_gc_trail"] = true; allowed["runtime_gc_trail_limited"] = true }

func (l *Logger) GCTrail(s GCTrail) {
	if s.valid() {
		s.Samples = append([]GCWindow(nil), s.Samples...)
		if s.Heartbeat != nil {
			heartbeat := *s.Heartbeat
			s.Heartbeat = &heartbeat
		}
		l.enqueueRecord(record{Code: "runtime_gc_trail", GCTrail: &s})
	}
}
