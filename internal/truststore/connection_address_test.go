package truststore

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAddressRecoveryPreservesLayoutAndRejectsStaleBinding(t *testing.T) {
	s := mustStore(t)
	peer := testPeer()
	if _, err := s.Import(peer, "peer", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	old := "192.168.1.2:45872"
	if _, err := s.SetConnection("peer", ConnectionSettings{Address: old, Edge: "RIGHT", RearmDistance: 128}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConnectionEdge("peer", peer.CertificateSHA256, "LEFT"); err != nil {
		t.Fatal(err)
	}
	p, err := s.SetConnectionAddress("peer", peer.CertificateSHA256, old, "192.168.1.3:45872")
	if err != nil || p.Edge != "LEFT" || p.RearmDistance != 128 || p.Fingerprint != peer.CertificateSHA256 {
		t.Fatalf("profile=%+v err=%v", p, err)
	}
	if _, err = s.SetConnectionAddress("peer", peer.CertificateSHA256, old, "192.168.1.4:45872"); !errors.Is(err, ErrConnectionChanged) {
		t.Fatalf("stale write: %v", err)
	}
	if _, err = s.SetConnectionAddress("peer", strings.Repeat("b", 64), p.Address, "192.168.1.4:45872"); err == nil {
		t.Fatal("changed peer accepted")
	}
	if _, err = s.SetConnectionAddress("peer", peer.CertificateSHA256, p.Address, "8.8.8.8:45872"); err == nil {
		t.Fatal("public address accepted")
	}
	if _, err = s.Revoke(peer.CertificateSHA256, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetConnectionAddress("peer", peer.CertificateSHA256, p.Address, "192.168.1.4:45872"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("revoked write: %v", err)
	}
}
