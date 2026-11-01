package truststore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
)

func TestConnectionWithPeerPreservesProfileAndCurrentTrust(t *testing.T) {
	store := mustStore(t)
	peer, now := testPeer(), time.Now().UTC()
	wantPeer, err := store.Import(peer, "target", false, now)
	if err != nil {
		t.Fatal(err)
	}
	wantProfile, err := store.SetConnection("target", ConnectionSettings{Address: "127.0.0.1:45872", Edge: "LEFT"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.directory, storeFilename))
	if err != nil {
		t.Fatal(err)
	}
	profile, record, err := store.ConnectionWithPeer("target")
	if err != nil || profile != wantProfile || !reflect.DeepEqual(record, wantPeer) {
		t.Fatal("profile and peer disagree", profile, record, err)
	}
	legacy, err := store.Connection("target")
	if err != nil || legacy != profile {
		t.Fatal("existing Connection contract changed", legacy, err)
	}
	after, err := os.ReadFile(filepath.Join(store.directory, storeFilename))
	if err != nil || string(after) != string(before) {
		t.Fatal("read modified trust", err)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ConnectionWithPeer("target"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatal("returned cached/revoked peer", err)
	}
}

func TestConnectionWithPeerPreservesMissingAmbiguousAndInvalidProfiles(t *testing.T) {
	store := mustStore(t)
	peer, now := testPeer(), time.Now().UTC()
	if _, err := store.Import(peer, "target", true, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ConnectionWithPeer("target"); !errors.Is(err, ErrConnectionNotConfigured) {
		t.Fatal("missing profile misclassified", err)
	}
	if _, _, err := store.ConnectionWithPeer("missing"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatal("missing peer misclassified", err)
	}
	if _, err := store.SetConnection("target", ConnectionSettings{Address: "127.0.0.1:45872", Edge: "RIGHT"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.directory, connectionFilename(peer.CertificateSHA256)), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ConnectionWithPeer("target"); err == nil {
		t.Fatal("accepted damaged profile")
	}
	peer.CertificateSHA256 = fmt.Sprintf("%064x", 1)
	if _, err := store.Import(peer, "target", false, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// Ambiguity must precede the damaged profile read, preserving duplicate rules.
	if _, _, err := store.ConnectionWithPeer("target"); !errors.Is(err, ErrAliasAmbiguous) {
		t.Fatal("duplicate alias resolution changed", err)
	}
	if _, _, err := (*Store)(nil).ConnectionWithPeer("target"); err == nil {
		t.Fatal("nil store accepted")
	}
}

func BenchmarkConnectionPeerLookup(b *testing.B) {
	for _, count := range []int{1, 1000} {
		b.Run(fmt.Sprintf("peers=%d", count), func(b *testing.B) {
			parent := b.TempDir()
			if _, err := peercredentials.ProtectSecurityDirectory(parent); err != nil {
				b.Fatal(err)
			}
			store, err := New(filepath.Join(parent, "trust"))
			if err != nil {
				b.Fatal(err)
			}
			peer, now := testPeer(), time.Now().UTC()
			if _, err := store.Import(peer, "target", false, now); err != nil {
				b.Fatal(err)
			}
			if _, err := store.SetConnection("target", ConnectionSettings{Address: "127.0.0.1:45872", Edge: "LEFT"}); err != nil {
				b.Fatal(err)
			}
			if err := store.mutate(false, now, func(doc *Document) error {
				for i := 1; i < count; i++ {
					doc.Peers = append(doc.Peers, PeerRecord{Fingerprint: fmt.Sprintf("%064x", i), Role: peer.Role, Alias: fmt.Sprintf("peer-%d", i), CreatedAt: now})
				}
				return nil
			}); err != nil {
				b.Fatal(err)
			}
			for _, combined := range []bool{false, true} {
				b.Run(fmt.Sprintf("combined=%t", combined), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var profile ConnectionProfile
						var record PeerRecord
						var err error
						if combined {
							profile, record, err = store.ConnectionWithPeer("target")
						} else {
							profile, err = store.Connection("target")
							if err == nil {
								var doc Document
								doc, err = store.List()
								for _, candidate := range doc.Peers {
									if candidate.Fingerprint == profile.Fingerprint && candidate.RevokedAt == nil {
										record = candidate
									}
								}
							}
						}
						if err != nil || profile.Fingerprint != peer.CertificateSHA256 || record.Role != peer.Role {
							b.Fatal("lookup failed", err)
						}
					}
				})
			}
		})
	}
}
