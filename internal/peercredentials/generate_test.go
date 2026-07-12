package peercredentials

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func TestGenerateCreatesSeparatedUsableIdentitiesWithoutCAKey(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	manifest, err := Generate(output, "peer.test", now)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if manifest.ExpiresAt.Sub(manifest.CreatedAt) != Validity {
		t.Fatalf("validity = %s", manifest.ExpiresAt.Sub(manifest.CreatedAt))
	}
	for _, role := range []string{"listener", "connector"} {
		for _, name := range []string{"ca.pem", "identity.pem", "identity-key.pem", "peer.pem"} {
			if _, err := os.Stat(filepath.Join(output, role, name)); err != nil {
				t.Fatalf("missing %s/%s: %v", role, name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(output, role, "ca-key.pem")); !os.IsNotExist(err) {
			t.Fatalf("CA private key unexpectedly persisted for %s", role)
		}
	}
	listener := loadIdentity(t, filepath.Join(output, "listener"))
	connector := loadIdentity(t, filepath.Join(output, "connector"))
	roots := loadRoots(t, filepath.Join(output, "listener", "ca.pem"))
	if _, err := transport.NewServerTLSConfig(listener, roots); err != nil {
		t.Fatalf("NewServerTLSConfig() error = %v", err)
	}
	if _, err := transport.NewClientTLSConfig(connector, roots, "peer.test"); err != nil {
		t.Fatalf("NewClientTLSConfig() error = %v", err)
	}
	listenerPeer, err := LoadTrustedPeer(filepath.Join(output, "listener", "peer.pem"))
	if err != nil || listenerPeer.Role != transport.AuthenticatedPeerConnector {
		t.Fatalf("listener trusted peer = %+v, %v", listenerPeer, err)
	}
	connectorPeer, err := LoadTrustedPeer(filepath.Join(output, "connector", "peer.pem"))
	if err != nil || connectorPeer.Role != transport.AuthenticatedPeerListener {
		t.Fatalf("connector trusted peer = %+v, %v", connectorPeer, err)
	}
}

func TestGenerateRefusesExistingOutputAndPreservesIt(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(output, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(output, "peer.test", time.Now()); err == nil {
		t.Fatal("Generate() overwrote an existing output directory")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("existing output changed: data=%q error=%v", data, err)
	}
}

func TestGenerateSupportsIPServerName(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	if _, err := Generate(output, "192.0.2.10", time.Now()); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	identity := loadIdentity(t, filepath.Join(output, "listener"))
	certificate, err := x509.ParseCertificate(identity.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := certificate.VerifyHostname("192.0.2.10"); err != nil {
		t.Fatalf("VerifyHostname() error = %v", err)
	}
}

func loadIdentity(t *testing.T, directory string) tls.Certificate {
	t.Helper()
	identity, err := tls.LoadX509KeyPair(filepath.Join(directory, "identity.pem"), filepath.Join(directory, "identity-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func loadRoots(t *testing.T, path string) *x509.CertPool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		t.Fatal("append CA")
	}
	return roots
}
