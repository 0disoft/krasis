package productidentity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
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
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(payload, after) {
			t.Fatal("corrupt document was replaced")
		}
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
