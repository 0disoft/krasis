package residentagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestUpdateInspectionNeedsTrustButNeverPreflightsOrStartsUpdate(t *testing.T) {
	peer := strings.Repeat("b", 64)
	store := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Fingerprint: peer, Alias: "peer"}}}}
	o := testOperationsWithTrust(t, store)
	o.updates = &peerupdate.Manager{}
	// A nil preflight forbids ALLOW but must not prevent read-only inspection.
	r := localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer}
	v, err := o.Update(context.Background(), r)
	if err != nil || v.Code != "peer_status" || v.PeerInfo == nil || v.PeerInfo.State != "UNKNOWN" {
		t.Fatalf("inspection: %+v %v", v, err)
	}
	now := time.Now()
	store.document.Peers[0].RevokedAt = &now
	v, err = o.Update(context.Background(), r)
	if err != nil || v.Code != "rejected" || v.PeerInfo != nil {
		t.Fatal("revoked peer inspected")
	}
}
