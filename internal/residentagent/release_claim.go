package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type releaseClaimLifetime struct{ timer *time.Timer }

// A claim shares the Connect/Pair/peer-update mutex. It never disconnects input.
// The request ID binds rollback to this owner; a crashed UI cannot fence forever.
func (o *Operations) ReleaseClaim(ctx context.Context, id string, generation int, revision string, claim bool) (localapi.OperationResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if ctx.Err() != nil || o.root.Err() != nil {
		return rejected("operation_cancelled"), nil
	}
	if !claim {
		if o.releaseClaimID != id || o.updateTargetGeneration != generation || o.updateTargetRevision != revision {
			return rejected("claim_changed"), nil
		}
		o.clearReleaseClaimLocked()
		return completed("release_released"), nil
	}
	if o.updateApplying || o.sessionDone != nil || o.sessionFaulted {
		return rejected("agent_busy"), nil
	}
	if p, ok := o.pairing.(interface{ Active() bool }); ok && p.Active() {
		return rejected("agent_busy"), nil
	}
	o.updateApplying, o.updateTargetGeneration, o.updateTargetRevision, o.releaseClaimID = true, generation, revision, id
	claimLifetime := &releaseClaimLifetime{}
	o.releaseClaim = claimLifetime
	claimLifetime.timer = time.AfterFunc(3*time.Minute, func() { o.expireReleaseClaim(claimLifetime) })
	return completed("release_claimed"), nil
}

func (o *Operations) expireReleaseClaim(claim *releaseClaimLifetime) {
	o.mu.Lock()
	defer o.mu.Unlock()
	// Stop cannot retract a callback already waiting for mu. An independent
	// lifetime also fences release/reacquire with the same external request ID.
	if o.releaseClaim == claim {
		o.clearReleaseClaimLocked()
	}
}

func (o *Operations) clearReleaseClaimLocked() {
	if o.releaseClaim != nil {
		o.releaseClaim.timer.Stop()
		o.releaseClaim = nil
	}
	o.updateApplying = false
	o.updateTargetGeneration = 0
	o.updateTargetRevision = ""
	o.releaseClaimID = ""
}
