package handoff

import (
	"context"
	"time"
)

const PipeOwnerProbeLimit = 2048
const MaxPipeOwnerCycles = uint64(1) << 48 // bounded, exactly representable JSON integer

// Probe only an actual tracked input in the existing 12-attempt/120s window.
// The extra native queries stop permanently at this collector's fixed budget.
// This decision affects diagnostics only, never authority or input outcomes.
func WantPipeOwnerCycles(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(sampleKey{}).(*sample)
	s := getSession(ctx)
	a := getAttempt(ctx)
	if v == nil || a == nil || v.vector.AuthorityMeasured&(1<<AuthorityPipeOwner) != 0 || s == nil || s.collector == nil || s.collector.stopped.Load() {
		return false
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if a.finished || time.Since(s.collector.started) >= Window {
		return false
	}
	for {
		used := s.collector.pipeOwnerProbes.Load()
		if used >= PipeOwnerProbeLimit {
			return false
		}
		if s.collector.pipeOwnerProbes.CompareAndSwap(used, used+1) {
			return true
		}
	}
}

// Record cycles only alongside the matching wall span. Measured zero remains
// distinguishable from absent/failed/exhausted probes; cycles are not time.
func RecordPipeOwnerCycles(ctx context.Context, cycles uint64) {
	if ctx == nil || cycles > MaxPipeOwnerCycles {
		return
	}
	v, _ := ctx.Value(sampleKey{}).(*sample)
	if v == nil || v.vector.PipeOwnerCyclesMeasured || v.vector.AuthorityMeasured&(1<<AuthorityPipeOwner) == 0 {
		return
	}
	v.vector.PipeOwnerCycles, v.vector.PipeOwnerCyclesMeasured = cycles, true
}

func ValidPipeOwnerCycles(cycles uint64, measured bool, authorityMeasured uint8) bool {
	if !measured {
		return cycles == 0
	}
	return cycles <= MaxPipeOwnerCycles && authorityMeasured&(1<<AuthorityPipeOwner) != 0
}
