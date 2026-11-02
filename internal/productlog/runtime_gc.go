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
	BeginMicros  int64      `json:"beginMicros"`
	EndMicros    int64      `json:"endMicros"`
	ReadMicros   int64      `json:"readMicros"`
	LateMicros   int64      `json:"lateMicros"`
	Known        uint16     `json:"known"`
	GCPauses     [6]uint64  `json:"gcPauses"`
	GCStopping   [6]uint64  `json:"gcStopping"`
	Runnable     [6]uint64  `json:"runnable"`
	Allocated    uint64     `json:"allocatedBytes"`
	Heap         uint64     `json:"heapBytes"`
	Cycles       uint64     `json:"cycles"`
	AssistMicros uint64     `json:"assistMicros"`
	GCCPUMicros  uint64     `json:"gcCPUMicros"`
	OS           GCOSWindow `json:"os,omitzero"`
}

// This is a bounded observation trail, never exact GC start/end timestamps.
// TriggerMicros is the monotonic callback-notification time in the agent run;
// TriggerObservedAt is the original failure or nonfatal observation's wall time.
// A sample ending after the trigger cannot be labelled as wholly preceding it.
// Fault and Observation are mutually exclusive; observations grant no authority
// and must never be presented as connection/service failures.
type GCTrail struct {
	Schema            uint8              `json:"schema"`
	TriggerMicros     int64              `json:"triggerMicros"`
	TriggerObservedAt string             `json:"triggerObservedAt"`
	Fault             string             `json:"fault"`
	Observation       string             `json:"observation,omitempty"`
	Wrapped           bool               `json:"wrapped"`
	SkippedTriggers   uint64             `json:"skippedTriggers"`
	Samples           []GCWindow         `json:"samples"`
	Heartbeat         *HeartbeatProgress `json:"heartbeat,omitempty"`
}

func (s GCTrail) valid() bool {
	if s.Heartbeat != nil && (!s.Heartbeat.valid() || s.Heartbeat.AtMicros != s.TriggerMicros) {
		return false
	}
	failure := s.Observation == "" && strings.HasPrefix(s.Fault, "fault_") && allowed[s.Fault]
	observation := s.Fault == "" && validGCObservation(s.Observation)
	if s.Schema != 1 || s.TriggerMicros < 1 || (!failure && !observation) || len(s.Samples) == 0 || len(s.Samples) > GCTraceLimit {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, s.TriggerObservedAt); err != nil || len(s.TriggerObservedAt) > 64 {
		return false
	}
	previous := int64(0)
	for _, w := range s.Samples {
		if !w.OS.valid() || w.BeginMicros < 1 || w.EndMicros <= w.BeginMicros || w.BeginMicros < previous || w.ReadMicros < 0 || w.ReadMicros > w.EndMicros-w.BeginMicros || w.OS.ReadMicros > uint64(w.EndMicros-w.BeginMicros) || w.LateMicros < 0 || w.Known & ^uint16(GCKnownMask) != 0 {
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

// Observations preserve nonfatal stalls without inventing a fault or a cause.
func validGCObservation(reason string) bool {
	switch reason {
	case "timer_late", "metrics_read_slow", "os_read_slow", "gc_pause_bucket", "gc_stopping_bucket", "runnable_bucket":
		return true
	}
	return false
}

func init() {
	allowed["runtime_gc_trail"] = true
	allowed["runtime_gc_trail_limited"] = true
	allowed["runtime_stall_trail_limited"] = true
}

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
