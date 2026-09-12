package productidentity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func TestTransportRejectsCorruptStoreWithoutReplacingIt(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, _, err := store.OpenOrCreate(now); err != nil {
		t.Fatal(err)
	}
	role := transport.AuthenticatedPeerListener
	if _, err := store.OpenTransport(role, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.directory, "transport-"+string(role)+".json")
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{
		[]byte("{"),
		append(append([]byte(nil), valid...), []byte(" {}")...),
		append([]byte(`{"unknown":true,`), valid[1:]...),
		bytes.Repeat([]byte("x"), maxStoreBytes+1),
	} {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.OpenTransport(role, now); err == nil {
			t.Fatal("corrupt document accepted")
		}
		if _, err := store.RenewTransport(role, strings.Repeat("a", 64), now); err == nil {
			t.Fatal("renewal overwrote corruption")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(payload, after) {
			t.Fatal("corrupt document was replaced")
		}
	}
}

func TestExplicitTransportRenewalUsesExpectedFingerprint(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	identity, _, err := store.OpenOrCreate(now)
	if err != nil {
		t.Fatal(err)
	}
	role := transport.AuthenticatedPeerListener
	if _, err := store.InspectTransport(role, now); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect created credential: %v", err)
	}
	old, err := store.OpenTransport(role, now)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.OpenTransport(transport.AuthenticatedPeerConnector, now)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.InspectTransport(role, old.Leaf.NotAfter)
	if err != nil || !info.Expired {
		t.Fatalf("expired inspection: %+v %v", info, err)
	}
	if _, err := store.RenewTransport(role, strings.Repeat("0", 64), now); !errors.Is(err, ErrTransportFingerprintChanged) {
		t.Fatalf("wrong fingerprint: %v", err)
	}
	stillOld, err := store.OpenTransport(role, now)
	if err != nil || !bytes.Equal(stillOld.Certificate[0], old.Certificate[0]) {
		t.Fatal("failed renewal changed certificate")
	}
	next, err := store.RenewTransport(role, info.Fingerprint, old.Leaf.NotAfter)
	if err != nil || next.Fingerprint == info.Fingerprint || next.Expired {
		t.Fatalf("renewal: %+v %v", next, err)
	}
	if _, err := store.RenewTransport(role, info.Fingerprint, old.Leaf.NotAfter); !errors.Is(err, ErrTransportFingerprintChanged) {
		t.Fatalf("stale retry: %v", err)
	}
	loaded, err := store.OpenTransport(role, old.Leaf.NotAfter)
	if err != nil || bytes.Equal(loaded.Leaf.RawSubjectPublicKeyInfo, old.Leaf.RawSubjectPublicKeyInfo) {
		t.Fatal("renewal did not persist a new key")
	}
	unchanged, err := store.OpenTransport(transport.AuthenticatedPeerConnector, now)
	if err != nil || !bytes.Equal(other.Certificate[0], unchanged.Certificate[0]) {
		t.Fatal("renewal changed the other role")
	}
	current, _, err := store.OpenOrCreate(now)
	if err != nil || current.Public() != identity.Public() {
		t.Fatal("renewal changed device identity")
	}
}

func TestTransportCredentialsPersistAndSeparateRoles(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	identity, _, err := store.OpenOrCreate(now)
	if err != nil {
		t.Fatal(err)
	}
	var previous []byte
	for _, role := range []transport.AuthenticatedPeerRole{transport.AuthenticatedPeerListener, transport.AuthenticatedPeerConnector} {
		cert, err := store.OpenTransport(role, now)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := store.OpenTransport(role, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(cert.Certificate[0], reopened.Certificate[0]) {
			t.Fatal("credential changed on reopen")
		}
		if bytes.Equal(cert.Leaf.PublicKey.(ed25519.PublicKey), identity.PublicKey()) {
			t.Fatal("transport reused device identity key")
		}
		if bytes.Equal(previous, cert.Leaf.PublicKey.(ed25519.PublicKey)) {
			t.Fatal("roles reused the same key")
		}
		previous = cert.Leaf.PublicKey.(ed25519.PublicKey)
		if cert.Leaf.IsCA || !cert.Leaf.NotAfter.Equal(now.Add(TransportLifetime)) {
			t.Fatal("unexpected authority or lifetime")
		}
		roots := x509.NewCertPool()
		roots.AddCert(cert.Leaf)
		usage := x509.ExtKeyUsageServerAuth
		if role == transport.AuthenticatedPeerConnector {
			usage = x509.ExtKeyUsageClientAuth
		}
		if _, err := cert.Leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, DNSName: cert.Leaf.DNSNames[0], KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
			t.Fatalf("explicit peer anchor failed: %v", err)
		}
	}
}

func TestTransportCredentialsRejectExpiryAndIdentityRotation(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, _, err := store.OpenOrCreate(now); err != nil {
		t.Fatal(err)
	}
	role := transport.AuthenticatedPeerListener
	first, err := store.OpenTransport(role, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenTransport(role, first.Leaf.NotAfter); !errors.Is(err, ErrTransportRenewalRequired) {
		t.Fatalf("expiry error = %v", err)
	}
	reopened, err := store.OpenTransport(role, now)
	if err != nil || !bytes.Equal(first.Certificate[0], reopened.Certificate[0]) {
		t.Fatal("expired credential was overwritten")
	}
	if _, err := store.Rotate(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenTransport(role, now.Add(time.Hour)); !errors.Is(err, ErrTransportRenewalRequired) {
		t.Fatalf("rotation error = %v", err)
	}
}

func TestTransportValidationRejectsSubstitutions(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	identity, _, err := store.OpenOrCreate(now)
	if err != nil {
		t.Fatal(err)
	}
	role := transport.AuthenticatedPeerListener
	document, err := createTransport(identity.Public(), role, now)
	if err != nil {
		t.Fatal(err)
	}
	other, err := createTransport(identity.Public(), role, now)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*transportDocument){
		"key":         func(d *transportDocument) { d.PrivateKey = other.PrivateKey },
		"certificate": func(d *transportDocument) { d.Certificate = other.Certificate },
		"schema":      func(d *transportDocument) { d.SchemaVersion = "unknown" },
		"identity":    func(d *transportDocument) { d.IdentityFingerprint = "unknown" },
		"generation":  func(d *transportDocument) { d.Generation++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := document
			mutate(&changed)
			if _, err := validateTransport(changed, identity.Public(), role, now); err == nil {
				t.Fatal("substitution accepted")
			}
		})
	}
	if _, err := validateTransport(document, identity.Public(), transport.AuthenticatedPeerConnector, now); err == nil {
		t.Fatal("wrong role accepted")
	}
	if _, err := store.OpenTransport(role, time.Time{}); err == nil {
		t.Fatal("zero time accepted")
	}
	if _, err := store.OpenTransport("../invalid", now); err == nil {
		t.Fatal("invalid role accepted")
	}
}
