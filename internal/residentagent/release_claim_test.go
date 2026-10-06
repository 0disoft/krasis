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
