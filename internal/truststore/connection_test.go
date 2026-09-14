package truststore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConnectionPersistsWithoutChangingTrust(t *testing.T) {
	store := mustStore(t)
	peer, now := testPeer(), time.Now().UTC()
	if _, err := store.Import(peer, "peer", false, now); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.directory, storeFilename))
	if err != nil {
		t.Fatal(err)
	}
	settings := ConnectionSettings{Address: "192.168.1.2:4567", Edge: "RIGHT"}
	profile, err := store.SetConnection("peer", settings)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(store.directory)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Connection("peer")
	if err != nil || loaded != profile || loaded.Fingerprint != peer.CertificateSHA256 {
		t.Fatalf("profile = %+v, %v", loaded, err)
	}
	settings.Edge = "LEFT"
	if _, err := store.SetConnection("peer", settings); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(store.directory, storeFilename))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("connection settings changed trust")
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Connection("peer"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("revoked read: %v", err)
	}
	if _, err := store.SetConnection("peer", settings); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("revoked write: %v", err)
	}
}

func TestEdgeEditPreservesAddressAndRejectsChangedIdentity(t *testing.T) {
	store := mustStore(t)
	peer := testPeer()
	if _, err := store.Import(peer, "peer", false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetConnectionEdge("peer", peer.CertificateSHA256, "LEFT"); !errors.Is(err, ErrConnectionNotConfigured) {
		t.Fatal("missing profile created")
	}
	settings := ConnectionSettings{Address: "127.0.0.1:4567", Edge: "RIGHT"}
	if _, err := store.SetConnection("peer", settings); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetConnectionEdge("peer", strings.Repeat("b", 64), "LEFT"); err == nil {
		t.Fatal("changed identity accepted")
	}
	profile, err := store.SetConnectionEdge("peer", peer.CertificateSHA256, "LEFT")
	if err != nil || profile.Address != settings.Address || profile.Edge != "LEFT" {
		t.Fatalf("edge edit: %+v %v", profile, err)
	}
	if _, err := store.SetConnectionEdge("peer", peer.CertificateSHA256, "UP"); err == nil {
		t.Fatal("invalid edge accepted")
	}
}

func TestConnectionRejectsAmbiguityMissingAndCorruption(t *testing.T) {
	store := mustStore(t)
	peer, now := testPeer(), time.Now().UTC()
	if _, err := store.Import(peer, "peer", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Connection("peer"); !errors.Is(err, ErrConnectionNotConfigured) {
		t.Fatalf("absent profile: %v", err)
	}
	settings := ConnectionSettings{Address: "127.0.0.1:4567", Edge: "LEFT"}
	if _, err := store.SetConnection("peer", settings); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.directory, connectionFilename(peer.CertificateSHA256))
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetConnection("peer", settings); err == nil {
		t.Fatal("corrupt file overwritten")
	}
	if _, err := store.Connection("peer"); err == nil {
		t.Fatal("corrupt file accepted")
	}
	peer.CertificateSHA256 = strings.Repeat("b", 64)
	if _, err := store.Import(peer, "peer", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetConnection("peer", settings); !errors.Is(err, ErrAliasAmbiguous) {
		t.Fatalf("ambiguous write: %v", err)
	}
	if _, err := store.Connection("peer"); !errors.Is(err, ErrAliasAmbiguous) {
		t.Fatalf("ambiguous read: %v", err)
	}
}

func TestConnectionSettingsValidateLocalNetworkBoundary(t *testing.T) {
	for _, address := range []string{"192.168.1.2:1234", "127.0.0.1:1234", "[::1]:1234", "[fd00::1]:1234"} {
		if err := (ConnectionSettings{Address: address, Edge: "LEFT"}).Validate(); err != nil {
			t.Errorf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"example.com:1234", "8.8.8.8:1234", "0.0.0.0:1234", "[::]:1234", "192.168.1.2:0", "192.168.1.2", "[ff02::1]:1234", "[fe80::1%12]:1234", "[::ffff:8.8.8.8]:1234"} {
		if err := (ConnectionSettings{Address: address, Edge: "LEFT"}).Validate(); err == nil {
			t.Errorf("accepted %s", address)
		}
	}
	if err := (ConnectionSettings{Address: "127.0.0.1:1234", Edge: "TOP"}).Validate(); err == nil {
		t.Fatal("accepted unsupported edge")
	}
}
