package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
)

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
