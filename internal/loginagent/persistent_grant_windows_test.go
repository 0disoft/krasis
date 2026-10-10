//go:build windows

package loginagent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPersistentLiveGrantKeepsOwnerChecksAfterCertificateExpiry(t *testing.T) {
	g, base, trust, identities := liveGrantFixture(t)
	now := time.Now().UTC()
	old, err := readGrantConnectionAt(g, base, now)
	if err != nil {
		t.Fatal(err)
	}
	g.Schema, g.ExpiresAt = PersistentGrantSchema, time.Time{}
	g.LocalTransport, err = transport.ProductKeyFingerprint(old.Local.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	after := now.Add(90 * 24 * time.Hour)
	live, err := readGrantConnectionAt(g, base, after)
	if err != nil || !live.ExpiresAt.IsZero() {
		t.Fatal("persistent read failed", err)
	}
	renewed, err := productidentity.RenewableTransport(live.Local, after)
	if err != nil || !renewed.Leaf.NotAfter.After(after) {
		t.Fatal("pre-login cannot renew expired credential", err)
	}
	if _, err := identities.RenewTransport(transport.AuthenticatedPeerConnector, g.LocalTransport, now); err == nil {
		t.Fatal("key pin confused with certificate pin")
	}
	if _, err := trust.Revoke(g.Peer, after); err != nil {
		t.Fatal(err)
	}
	if _, err := readGrantConnectionAt(g, base, after); err == nil {
		t.Fatal("persistent grant ignored trust revocation")
	}
}

func TestPersistentGrantRejectsLocalKeyReplacement(t *testing.T) {
	g, base, _, identities := liveGrantFixture(t)
	now := time.Now().UTC()
	live, err := readGrantConnectionAt(g, base, now)
	if err != nil {
		t.Fatal(err)
	}
	g.Schema, g.ExpiresAt = PersistentGrantSchema, time.Time{}
	g.LocalTransport, err = transport.ProductKeyFingerprint(live.Local.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := transport.AuthenticatedPeerFromCertificate(live.Local.Leaf)
	if _, err := identities.RenewTransport(actual.Role, actual.CertificateSHA256, now); err != nil {
		t.Fatal(err)
	}
	if _, err := readGrantConnectionAt(g, base, now); err == nil {
		t.Fatal("new unapproved role key inherited privileged grant")
	}
}
