package truststore

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/transport"
)

func replacementCertificate(t *testing.T, now time.Time) (transport.AuthenticatedPeerContext, tls.Certificate) {
	t.Helper()
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
	return peer, certificate
}

func TestReconfirmationPreservesExactExistingGrant(t *testing.T) {
	now := time.Now().UTC()
	peer, certificate := replacementCertificate(t, now)
	for _, scenario := range []string{"same", "alias", "capability", "role", "expired", "revoked", "missing_certificate", "damaged_certificate"} {
		t.Run(scenario, func(t *testing.T) {
			store := mustStore(t)
			original, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", false, now)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := store.SetConnection("peer", ConnectionSettings{Address: "127.0.0.1:12345", Edge: "LEFT"})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "revoked" {
				if _, err := store.Revoke(peer.CertificateSHA256, now); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(store.directory, peerCertificateFilename(peer.CertificateSHA256))
			if scenario == "missing_certificate" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "damaged_certificate" {
				if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := store.List()
			if err != nil {
				t.Fatal(err)
			}
			alias, allow, at, expected := "peer", false, now, peer
			switch scenario {
			case "alias":
				alias = "different"
			case "capability":
				allow = true
			case "role":
				expected.Role = transport.AuthenticatedPeerConnector
			case "expired":
				at = certificate.Leaf.NotAfter
			}
			record, err := store.ReconfirmWithCertificate(expected, certificate.Certificate[0], alias, allow, at)
			if scenario == "same" {
				if err != nil || !reflect.DeepEqual(record, original) {
					t.Fatalf("same grant: %+v %v", record, err)
				}
			} else if err == nil {
				t.Fatal("changed or unavailable grant was accepted")
			}
			after, err := store.List()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("reconfirmation mutated trust: %v", err)
			}
			if scenario != "revoked" {
				loaded, err := store.Connection("peer")
				if err != nil || loaded != profile {
					t.Fatalf("reconfirmation changed profile: %v", err)
				}
			}
			if scenario == "missing_certificate" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing certificate was recreated")
				}
			}
			if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", false, now); !errors.Is(err, ErrPeerExists) {
				t.Fatalf("raw import semantics changed: %v", err)
			}
		})
	}
}

func TestCertificateReplacementPreservesOldTrustUntilCommit(t *testing.T) {
	now := time.Now().UTC()
	old, oldCertificate := replacementCertificate(t, now)
	next, nextCertificate := replacementCertificate(t, now)
	for _, failure := range []string{"mismatch", "expired", "certificate_write", "prepared_only", "revoked", "duplicate"} {
		t.Run(failure, func(t *testing.T) {
			store := mustStore(t)
			if _, err := store.ImportWithCertificate(old, oldCertificate.Certificate[0], "peer", true, now); err != nil {
				t.Fatal(err)
			}
			settings := ConnectionSettings{Address: "127.0.0.1:12345", Edge: "RIGHT"}
			profile, err := store.SetConnection("peer", settings)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "revoked" {
				if _, err := store.Revoke(old.CertificateSHA256, now); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "duplicate" {
				if _, err := store.ImportWithCertificate(next, nextCertificate.Certificate[0], "other", false, now); err != nil {
					t.Fatal(err)
				}
			}
			before, err := store.List()
			if err != nil {
				t.Fatal(err)
			}
			der, at := nextCertificate.Certificate[0], now
			if failure == "mismatch" {
				der = oldCertificate.Certificate[0]
			}
			if failure == "expired" {
				at = nextCertificate.Leaf.NotAfter
			}
			if failure == "certificate_write" {
				if err := os.Mkdir(filepath.Join(store.directory, peerCertificateFilename(next.CertificateSHA256)), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			var revoked, added PeerRecord
			if failure == "prepared_only" {
				revoked, added, err = store.replacePeer(old.CertificateSHA256, next, "peer", false, now, func(directory *securefs.Directory) error {
					if err := persistPeerCertificate(directory, next.CertificateSHA256, der); err != nil {
						return err
					}
					return errors.New("stop before trust commit")
				})
			} else {
				revoked, added, err = store.ReplaceWithCertificate(old.CertificateSHA256, next, der, "peer", false, at)
			}
			if err == nil || revoked.Fingerprint != "" || added.Fingerprint != "" {
				t.Fatalf("failed replacement reported records: %v", err)
			}
			after, err := store.List()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("failed replacement changed trust: %v", err)
			}
			if failure != "revoked" {
				if _, err := store.PeerCertificate(old, now); err != nil {
					t.Fatalf("old certificate lost: %v", err)
				}
				loaded, err := store.Connection("peer")
				if err != nil || loaded != profile {
					t.Fatalf("old settings lost: %v", err)
				}
			}
			if failure == "prepared_only" {
				if _, err := store.PeerCertificate(next, now); !errors.Is(err, ErrPeerNotFound) {
					t.Fatalf("orphan granted trust: %v", err)
				}
				if _, _, err := store.ReplaceWithCertificate(old.CertificateSHA256, next, der, "peer", false, now); err != nil {
					t.Fatalf("retry failed: %v", err)
				}
				after, err = store.List()
				if err != nil || after.Revision != before.Revision+1 {
					t.Fatalf("replacement revision: %v", err)
				}
				if _, err := store.PeerCertificate(old, now); !errors.Is(err, ErrPeerRevoked) {
					t.Fatalf("old trust survived: %v", err)
				}
				leaf, err := store.PeerCertificate(next, now)
				if err != nil || !leaf.Equal(nextCertificate.Leaf) {
					t.Fatalf("new certificate unavailable: %v", err)
				}
				if _, err := store.Connection("peer"); !errors.Is(err, ErrConnectionNotConfigured) {
					t.Fatalf("settings implicitly transferred: %v", err)
				}
			}
		})
	}
}

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
