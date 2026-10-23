package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestOldPeerUpdatePreparationCannotAuthorizeAnotherClaim(t *testing.T) {
	o := testOperations(t, nil)
	t.Cleanup(func() { _ = o.Close(context.Background()) })
	old := &releaseClaimLifetime{}
	if r, _ := o.ReleaseClaim(context.Background(), strings.Repeat("a", 32), 999, strings.Repeat("b", 40), true); r.Code != "release_claimed" {
		t.Fatal(r)
	}
	if o.finishPeerUpdatePreparation(old, true) {
		t.Fatal("obsolete peer preparation still authorized installer launch")
	}
	current := o.releaseClaim
	if o.finishPeerUpdatePreparation(old, false) || o.releaseClaim != current || !o.updateApplying {
		t.Fatal("obsolete preparation failure cleared the new claim")
	}
}

func TestPeerUpdateExpiryCannotActAfterClaimReplacement(t *testing.T) {
	o := testOperations(t, nil)
	t.Cleanup(func() { _ = o.Close(context.Background()) })
	expired := 0
	old := &releaseClaimLifetime{timer: time.NewTimer(time.Hour), peerExpiry: func() bool { expired++; return true }}
	o.releaseClaim, o.updateApplying = old, true
	o.clearReleaseClaimLocked()
	current := &releaseClaimLifetime{timer: time.NewTimer(time.Hour), peerExpiry: func() bool { expired++; return true }}
	o.releaseClaim, o.updateApplying = current, true
	o.expireReleaseClaim(old)
	if expired != 0 || o.releaseClaim != current || !o.updateApplying {
		t.Fatal("old peer expiry touched replacement", expired)
	}
	o.expireReleaseClaim(current)
	o.expireReleaseClaim(current)
	if expired != 1 || o.releaseClaim != nil || o.updateApplying || current.timer.Stop() || old.timer.Stop() {
		t.Fatal("expiry did not retire exactly one owned claim and its timer", expired)
	}
}

func TestCurrentPeerPreparationFailureReleasesOnlyItsLifetime(t *testing.T) {
	o := testOperations(t, nil)
	o.updates = &peerupdate.Manager{}
	claim := &releaseClaimLifetime{timer: time.NewTimer(time.Hour)}
	o.releaseClaim, o.updateApplying = claim, true
	o.updateTargetGeneration, o.updateTargetRevision = 999, "revision"
	if !o.finishPeerUpdatePreparation(claim, false) || o.releaseClaim != nil || o.updateApplying || o.updateTargetGeneration != 0 || o.updateTargetRevision != "" || claim.timer.Stop() {
		t.Fatal("current failed preparation retained its fence or timeout")
	}
}

func TestReleaseClaimFencesConnectPairAndRejectsForeignRollback(t *testing.T) {
	o := testOperations(t, nil)
	ctx, id, rev := context.Background(), strings.Repeat("a", 32), strings.Repeat("b", 40)
	r, err := o.ReleaseClaim(ctx, id, 999, rev, true)
	if err != nil || r.Code != "release_claimed" {
		t.Fatal(r, err)
	}
	if r, _ = o.Connect(ctx, localapi.OperationRequest{PeerAlias: "peer"}); r.Code != "update_in_progress" {
		t.Fatal(r)
	}
	if r, _ = o.Pair(ctx, localapi.OperationRequest{}); r.Code != "update_in_progress" {
		t.Fatal(r)
	}
	if r, _ = o.ReleaseClaim(ctx, strings.Repeat("c", 32), 999, rev, false); r.Code != "claim_changed" || !o.updateApplying {
		t.Fatal(r)
	}
	if r, _ = o.ReleaseClaim(ctx, id, 999, rev, false); r.Code != "release_released" || o.updateApplying {
		t.Fatal(r)
	}
	o.sessionDone = make(chan struct{})
	if r, _ = o.ReleaseClaim(ctx, id, 999, rev, true); r.Code != "agent_busy" {
		t.Fatal(r)
	}
}

func TestReleaseClaimOldExpiryCannotReleaseReusedRequestID(t *testing.T) {
	o := testOperations(t, nil)
	t.Cleanup(func() { _ = o.Close(context.Background()) })
	ctx, id, rev := context.Background(), strings.Repeat("a", 32), strings.Repeat("b", 40)
	if r, _ := o.ReleaseClaim(ctx, id, 999, rev, true); r.Code != "release_claimed" {
		t.Fatal(r)
	}
	old := o.releaseClaim
	if r, _ := o.ReleaseClaim(ctx, id, 999, rev, false); r.Code != "release_released" {
		t.Fatal(r)
	}
	if old.timer.Stop() {
		t.Fatal("released claim retained a live timer")
	}
	if r, _ := o.ReleaseClaim(ctx, id, 999, rev, true); r.Code != "release_claimed" {
		t.Fatal(r)
	}
	current := o.releaseClaim
	o.expireReleaseClaim(old) // Model a callback already running when Stop occurred.
	if !o.updateApplying || o.releaseClaim != current {
		t.Fatal("old expiry cleared a new claim with the same ID")
	}
	o.expireReleaseClaim(current)
	if o.updateApplying || o.releaseClaim != nil || o.releaseClaimID != "" || current.timer.Stop() {
		t.Fatal("current expiry did not clear its claim and timer")
	}
}

func TestReleaseClaimShutdownStopsTimer(t *testing.T) {
	o := testOperations(t, nil)
	if r, _ := o.ReleaseClaim(context.Background(), strings.Repeat("a", 32), 999, strings.Repeat("b", 40), true); r.Code != "release_claimed" {
		t.Fatal(r)
	}
	claim := o.releaseClaim
	if err := o.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.releaseClaim != nil || o.updateApplying || claim.timer.Stop() {
		t.Fatal("shutdown retained the claim or timer")
	}
	o.expireReleaseClaim(claim) // Late callbacks are harmless after shutdown too.
}
