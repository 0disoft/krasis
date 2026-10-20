package handoff

import (
	"context"
	"time"
)

// Client-local subspans of ClientAuthority, not new wire or authority fields.
type AuthorityStage uint8

// ForkAuthority gives a bounded native query its own diagnostic sample. The
// caller merges only after joining it; an expired query never writes a sample
// that its former caller may already have finalized.
func ForkAuthority(ctx context.Context) (context.Context, func()) {
	parent, _ := ctx.Value(sampleKey{}).(*sample)
	if parent == nil {
		return ctx, func() {}
	}
	child := *parent
	return context.WithValue(ctx, sampleKey{}, &child), func() {
		for stage := AuthorityStage(0); stage < AuthorityStageCount; stage++ {
			bit := uint8(1 << stage)
			if parent.vector.AuthorityMeasured&bit == 0 && child.vector.AuthorityMeasured&bit != 0 {
				parent.vector.AuthorityMicros[stage] = child.vector.AuthorityMicros[stage]
				parent.vector.AuthorityMeasured |= bit
			}
		}
		if !parent.vector.PipeOwnerCyclesMeasured && child.vector.PipeOwnerCyclesMeasured {
			parent.vector.PipeOwnerCycles = child.vector.PipeOwnerCycles
			parent.vector.PipeOwnerCyclesMeasured = true
		}
	}
}

const (
	AuthorityLock AuthorityStage = iota
	AuthorityPipeOwner
	AuthorityServiceStatus
	AuthorityStageCount
)

func ValidAuthorityDetail(micros [3]uint32, measured uint8) bool {
	if measured>>AuthorityStageCount != 0 {
		return false
	}
	for i, n := range micros {
		if n > uint32(Window.Microseconds()) || (measured&(1<<i) == 0 && n != 0) {
			return false
		}
	}
	return true
}

// RecordAuthority attaches only to one actual input sample. Bootstrap and idle
// maintenance have no sample. Recovery cannot replace the first measured call.
func RecordAuthority(ctx context.Context, stage AuthorityStage, elapsed time.Duration) {
	if ctx == nil || stage >= AuthorityStageCount {
		return
	}
	v, _ := ctx.Value(sampleKey{}).(*sample)
	if v == nil || v.vector.AuthorityMeasured&(1<<stage) != 0 {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
	}
	v.vector.AuthorityMicros[stage] = uint32(elapsed.Microseconds())
	v.vector.AuthorityMeasured |= 1 << stage
}
