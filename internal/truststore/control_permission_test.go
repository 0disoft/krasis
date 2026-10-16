package truststore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestControlPermissionRereadsPolicyCertificateAndRevocation(t *testing.T) {
	now := time.Now().UTC()
	peer, certificate := replacementCertificate(t, now)
	for _, denied := range []bool{false, true} {
		store := mustStore(t)
		if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", !denied, now); err != nil {
			t.Fatal(err)
		}
		if allowed, err := store.InboundControlAllowed(peer, now); err != nil || allowed == denied {
			t.Fatal("inbound policy changed", allowed, err)
		}
		if _, err := store.Revoke(peer.CertificateSHA256, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if allowed, err := store.InboundControlAllowed(peer, now); allowed || !errors.Is(err, ErrPeerRevoked) {
			t.Fatal("revoked peer reused earlier verdict", allowed, err)
		}
	}
	store := mustStore(t)
	if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", true, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.directory, peerCertificateFilename(peer.CertificateSHA256))); err != nil {
		t.Fatal(err)
	}
	if allowed, err := store.InboundControlAllowed(peer, now); allowed || err == nil {
		t.Fatal("missing certificate reused document permission", allowed, err)
	}
}
