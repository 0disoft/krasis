package truststore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
)

func TestCertificateWriteFailureDoesNotCommitTrust(t *testing.T) {
	now := time.Now().UTC()
	paths, _, _, err := productidentity.BootstrapWindows(t.TempDir(), now)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := productidentity.New(paths.Identity)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := identities.OpenTransport(transport.AuthenticatedPeerListener, now)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(certificate.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	store := mustStore(t)
	if err := store.prepareDirectory(true); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(store.directory, peerCertificateFilename(peer.CertificateSHA256)), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", true, now); err == nil {
		t.Fatal("unwritable certificate accepted")
	}
	if _, err := store.List(); !errors.Is(err, ErrStoreNotFound) {
		t.Fatalf("partial trust registration: %v", err)
	}
}

func TestUncommittedCertificateHasNoTrustAndCanBeRetried(t *testing.T) {
	now := time.Now().UTC()
	paths, _, _, err := productidentity.BootstrapWindows(t.TempDir(), now)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := productidentity.New(paths.Identity)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := identities.OpenTransport(transport.AuthenticatedPeerListener, now)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(certificate.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	store := mustStore(t)
	_, directory, release, err := store.openLocked(true)
	if err != nil {
		t.Fatal(err)
	}
	err = persistPeerCertificate(directory, peer.CertificateSHA256, certificate.Certificate[0])
	release()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PeerCertificate(peer, now); !errors.Is(err, ErrStoreNotFound) {
		t.Fatalf("orphan granted trust: %v", err)
	}
	if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", false, now); err != nil {
		t.Fatalf("retry: %v", err)
	}
	loaded, err := store.PeerCertificate(peer, now)
	if err != nil || !loaded.Equal(certificate.Leaf) {
		t.Fatalf("retry load: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.directory, peerCertificateFilename(peer.CertificateSHA256)), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PeerCertificate(peer, now); err == nil {
		t.Fatal("corrupt certificate accepted")
	}
}
