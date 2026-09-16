package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/truststore"
	"testing"
	"time"
)

func TestManagementAuthorizationIndependentOfInputState(t *testing.T) {
	trust := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "B", Fingerprint: "paired"}}}}
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := &Operations{root: root, trust: trust}
	for _, state := range []string{"LOCAL", "CONNECTED", "RECONNECTING", "FAULTED"} {
		o.sessionState = state
		o.sessionFaulted = state == "FAULTED"
		if !o.updateAuthorized(root, "B", "paired") {
			t.Fatalf("management blocked in %s", state)
		}
	}
	if o.updateAuthorized(root, "other", "unknown") {
		t.Fatal("unpaired peer authorized")
	}
	o.updateApplying = true
	if o.updateAuthorized(root, "B", "paired") {
		t.Fatal("apply fence ignored")
	}
	o.updateApplying = false
	now := time.Now()
	trust.document.Peers[0].RevokedAt = &now
	if o.updateAuthorized(root, "B", "paired") {
		t.Fatal("revocation ignored")
	}
	trust.document.Peers[0].RevokedAt = nil
	cancel()
	if o.updateAuthorized(root, "B", "paired") {
		t.Fatal("shutdown ignored")
	}
}
