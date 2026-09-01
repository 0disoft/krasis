package pairing

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestPairingRequiresMatchingLocalAndRemoteConfirmation(t *testing.T) {
	now := time.Date(2026, time.September, 1, 1, 2, 3, 0, time.UTC)
	initiator := testIdentity(t, "initiator", now)
	responder := testIdentity(t, "responder", now)
	offer, err := createOffer(initiator, testPeer("a", transport.AuthenticatedPeerListener), now, bytes.NewReader(bytes.Repeat([]byte{0x2a}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	responderSession, response, err := Accept(offer, responder, testPeer("b", transport.AuthenticatedPeerConnector), now)
	if err != nil {
		t.Fatal(err)
	}
	initiatorSession, err := Complete(offer, response, initiator, now)
	if err != nil {
		t.Fatal(err)
	}
	if initiatorSession.ConfirmationCode() != responderSession.ConfirmationCode() {
		t.Fatalf("codes differ: %q != %q", initiatorSession.ConfirmationCode(), responderSession.ConfirmationCode())
	}
	if _, err := initiatorSession.Confirm("000000", now); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("Confirm() error = %v", err)
	}
	initiatorReceipt, err := initiatorSession.Confirm(initiatorSession.ConfirmationCode(), now)
	if err != nil {
		t.Fatal(err)
	}
	store := testTrustStore(t)
	if _, err := responderSession.Finalize(initiatorReceipt, store, "initiator", true, now); !errors.Is(err, ErrLocalConfirmation) {
		t.Fatalf("Finalize() without local confirmation error = %v", err)
	}
	responderReceipt, err := responderSession.Confirm(responderSession.ConfirmationCode(), now)
	if err != nil {
		t.Fatal(err)
	}
	record, err := responderSession.Finalize(initiatorReceipt, store, "initiator", true, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.Fingerprint != offer.Initiator.TransportPeer.CertificateSHA256 || !record.AllowInboundControl {
		t.Fatalf("record = %+v", record)
	}
	initiatorStore := testTrustStore(t)
	if _, err := initiatorSession.Finalize(responderReceipt, initiatorStore, "responder", false, now); err != nil {
		t.Fatal(err)
	}
}

func TestPairingRejectsExpiredTamperedAndWrongSessionReceipts(t *testing.T) {
	now := time.Date(2026, time.September, 1, 1, 2, 3, 0, time.UTC)
	initiator := testIdentity(t, "initiator", now)
	responder := testIdentity(t, "responder", now)
	offer, err := createOffer(initiator, testPeer("a", transport.AuthenticatedPeerListener), now, bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	responderSession, response, err := Accept(offer, responder, testPeer("b", transport.AuthenticatedPeerConnector), now)
	if err != nil {
		t.Fatal(err)
	}
	initiatorSession, err := Complete(offer, response, initiator, now)
	if err != nil {
		t.Fatal(err)
	}
	initiatorReceipt, err := initiatorSession.Confirm(initiatorSession.ConfirmationCode(), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := responderSession.Confirm(responderSession.ConfirmationCode(), now); err != nil {
		t.Fatal(err)
	}
	tampered := initiatorReceipt
	tampered.Signature = append([]byte(nil), tampered.Signature...)
	tampered.Signature[0] ^= 0xff
	if _, err := responderSession.Finalize(tampered, testTrustStore(t), "initiator", true, now); !errors.Is(err, ErrRemoteConfirmation) {
		t.Fatalf("tampered Finalize() error = %v", err)
	}
	if _, err := responderSession.Finalize(initiatorReceipt, testTrustStore(t), "initiator", true, offer.ExpiresAt.Add(time.Nanosecond)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired Finalize() error = %v", err)
	}
	response.Responder.IdentityPublicKey[0] ^= 0xff
	if _, err := Complete(offer, response, initiator, now); err == nil {
		t.Fatal("Complete() accepted a tampered response")
	}
}

func testIdentity(t *testing.T, name string, now time.Time) *productidentity.Identity {
	t.Helper()
	parent := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := peercredentials.ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	store, err := productidentity.New(filepath.Join(parent, "identity"))
	if err != nil {
		t.Fatal(err)
	}
	identity, _, err := store.OpenOrCreate(now)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func testTrustStore(t *testing.T) *truststore.Store {
	t.Helper()
	parent := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	store, err := truststore.New(filepath.Join(parent, "trust"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testPeer(value string, role transport.AuthenticatedPeerRole) transport.AuthenticatedPeerContext {
	return transport.AuthenticatedPeerContext{CertificateSHA256: strings.Repeat(value, 64), Role: role}
}
