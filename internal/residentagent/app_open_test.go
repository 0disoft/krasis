package residentagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestPeerUIOpenRequiresInboundPermissionAndLiveTrust(t *testing.T) {
	peer := strings.Repeat("a", 64)
	trust := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "A", Fingerprint: peer}}}}
	o := &Operations{root: context.Background(), trust: trust, localAppData: t.TempDir()}
	if code := o.openUIForPeer(peer); code != "rejected" {
		t.Fatal(code)
	}
	trust.document.Peers[0].AllowInboundControl = true
	if code := o.openUIForPeer(peer); code != "launch_record_missing" {
		t.Fatal(code)
	}
	now := time.Now()
	trust.document.Peers[0].RevokedAt = &now
	if code := o.openUIForPeer(peer); code != "rejected" {
		t.Fatal(code)
	}
	trust.document.Peers[0].RevokedAt = nil
	o.updateApplying = true
	if code := o.openUIForPeer(peer); code != "rejected" {
		t.Fatal(code)
	}
}
