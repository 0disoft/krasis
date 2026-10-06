package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"time"
)

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
	time.AfterFunc(3*time.Minute, func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.releaseClaimID == id {
			o.clearReleaseClaimLocked()
		}
	})
	return completed("release_claimed"), nil
}

func (o *Operations) clearReleaseClaimLocked() {
	o.updateApplying = false
	o.updateTargetGeneration = 0
	o.updateTargetRevision = ""
	o.releaseClaimID = ""
}
