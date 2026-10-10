package truststore

import (
	"errors"
	"testing"
	"time"
)

func TestKeyAnchorSurvivesLeafExpiryButNotRevocation(t *testing.T) {
	now := time.Now().UTC()
	peer, cert := replacementCertificate(t, now)
	store := mustStore(t)
	if _, err := store.ImportWithCertificate(peer, cert.Certificate[0], "B", true, now); err != nil {
		t.Fatal(err)
	}
	after := cert.Leaf.NotAfter.Add(time.Hour)
	if _, err := store.PeerCertificate(peer, after); err == nil {
		t.Fatal("legacy certificate expiry was relaxed")
	}
	anchor, err := store.PeerKeyAnchor(peer, after)
	if err != nil || !anchor.Equal(cert.Leaf) {
		t.Fatal("paired key anchor lost", err)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, after); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PeerKeyAnchor(peer, after); !errors.Is(err, ErrPeerRevoked) {
		t.Fatal("revocation ignored", err)
	}
}
