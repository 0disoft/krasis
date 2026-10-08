package residentagent

import (
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestUpdateApplyRejectionsExposeFixedReason(t *testing.T) {
	for _, reason := range []string{"preflight_failed", "busy", "session_changed", "claim_failed"} {
		t.Run(reason, func(t *testing.T) {
			fp := strings.Repeat("b", 64)
			o := testOperationsWithTrust(t, &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Fingerprint: fp, Alias: "B"}}}})
			o.updates = &peerupdate.Manager{}
			if reason != "preflight_failed" {
				o.preflight = func(string) error { return nil }
			}
			if reason == "busy" {
				o.updateApplying = true
			}
			if reason == "session_changed" {
				o.sessionDone = make(chan struct{})
				o.peerAlias = "other"
			}
			r, err := o.Update(context.Background(), localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "APPLY", Peer: fp, ID: strings.Repeat("c", 32)})
			if err != nil || r.Code != "apply_"+reason || o.counters["peer_update_apply_"+reason] != 1 || r.Directory != "" {
				t.Fatalf("rejection missing or exposed a launch: %+v %v", r, err)
			}
			if reason == "busy" && !o.updateApplying {
				t.Fatal("busy request released another claim")
			}
		})
	}
}
