package handoff

import (
	"context"
	"time"
)

// Client-local subspans of ClientAuthority, not new wire or authority fields.
type AuthorityStage uint8

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
