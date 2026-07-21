package peercredentials

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
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
		if err := ValidatePrivateKeySecurity(filepath.Join(output, role, "identity-key.pem")); err != nil {
			t.Fatalf("private key security for %s: %v", role, err)
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
	listenerPeer, err := loadTrustedPeerAt(filepath.Join(output, "listener", "peer.pem"), now)
	if err != nil || listenerPeer.Role != transport.AuthenticatedPeerConnector {
		t.Fatalf("listener trusted peer = %+v, %v", listenerPeer, err)
	}
	connectorPeer, err := loadTrustedPeerAt(filepath.Join(output, "connector", "peer.pem"), now)
	if err != nil || connectorPeer.Role != transport.AuthenticatedPeerListener {
		t.Fatalf("connector trusted peer = %+v, %v", connectorPeer, err)
	}
}

func TestLoadRejectsIdentityOutsideCertificateValidity(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	if _, err := Generate(output, "peer.test", now); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	directory := filepath.Join(output, "listener")
	caPath := filepath.Join(directory, "ca.pem")
	certificatePath := filepath.Join(directory, "identity.pem")
	keyPath := filepath.Join(directory, "identity-key.pem")

	if _, _, err := loadAt(caPath, certificatePath, keyPath, now.Add(-2*time.Minute)); !errors.Is(err, ErrCertificateNotYetValid) {
		t.Fatalf("loadAt() before validity error = %v, want ErrCertificateNotYetValid", err)
	}
	if _, _, err := loadAt(caPath, certificatePath, keyPath, now.Add(Validity+time.Second)); !errors.Is(err, ErrCertificateExpired) {
		t.Fatalf("loadAt() after validity error = %v, want ErrCertificateExpired", err)
	}
	if _, _, err := loadAt(caPath, certificatePath, keyPath, now); err != nil {
		t.Fatalf("loadAt() during validity error = %v", err)
	}
}

func TestValidateIdentityValidThroughRejectsInsufficientLifetime(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	if _, err := Generate(output, "peer.test", now); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	directory := filepath.Join(output, "listener")
	identity, roots, err := loadAt(
		filepath.Join(directory, "ca.pem"),
		filepath.Join(directory, "identity.pem"),
		filepath.Join(directory, "identity-key.pem"),
		now,
	)
	if err != nil {
		t.Fatalf("loadAt() error = %v", err)
	}
	deadline := identity.Leaf.NotAfter
	if err := ValidateIdentityValidThrough(identity, roots, deadline); err != nil {
		t.Fatalf("ValidateIdentityValidThrough() at NotAfter error = %v", err)
	}
	if err := ValidateIdentityValidThrough(identity, roots, deadline.Add(time.Nanosecond)); !errors.Is(err, ErrCertificateLifetimeInsufficient) {
		t.Fatalf("ValidateIdentityValidThrough() error = %v, want ErrCertificateLifetimeInsufficient", err)
	}
	if err := ValidateIdentityValidThrough(tls.Certificate{}, roots, deadline); err == nil {
		t.Fatal("ValidateIdentityValidThrough() accepted identity without parsed leaf")
	}
	if err := ValidateIdentityValidThrough(identity, nil, deadline); err == nil {
		t.Fatal("ValidateIdentityValidThrough() accepted identity without roots")
	}
	if err := ValidateIdentityValidThrough(identity, roots, time.Time{}); err == nil {
		t.Fatal("ValidateIdentityValidThrough() accepted zero required time")
	}
}

func TestLoadTrustedPeerRejectsCertificateOutsideValidity(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	if _, err := Generate(output, "peer.test", now); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	peerPath := filepath.Join(output, "listener", "peer.pem")

	if _, err := loadTrustedPeerAt(peerPath, now.Add(-2*time.Minute)); !errors.Is(err, ErrCertificateNotYetValid) {
		t.Fatalf("loadTrustedPeerAt() before validity error = %v, want ErrCertificateNotYetValid", err)
	}
	if _, err := loadTrustedPeerAt(peerPath, now.Add(Validity+time.Second)); !errors.Is(err, ErrCertificateExpired) {
		t.Fatalf("loadTrustedPeerAt() after validity error = %v, want ErrCertificateExpired", err)
	}
}

func TestValidatePrivateKeySecurityRejectsCopiedDirectoryUntilProtected(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "role")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "identity-key.pem")
	if err := os.WriteFile(keyPath, []byte("test-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateKeySecurity(keyPath); err == nil {
		t.Fatal("ValidatePrivateKeySecurity() accepted inherited directory permissions")
	}
	if err := ProtectPrivateKeyDirectory(directory); err != nil {
		t.Fatalf("ProtectPrivateKeyDirectory() error = %v", err)
	}
	if err := ValidatePrivateKeySecurity(keyPath); err != nil {
		t.Fatalf("ValidatePrivateKeySecurity() after protect = %v", err)
	}
}

func TestProtectPrivateKeyDirectoryRejectsNonRegularKey(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "role")
	if err := os.MkdirAll(filepath.Join(directory, "identity-key.pem"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ProtectPrivateKeyDirectory(directory); err == nil {
		t.Fatal("ProtectPrivateKeyDirectory() accepted a directory as the private key")
	}
}

func TestLoadRejectsInsecurePrivateKeyBeforeParsingCredentials(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "role")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ca.pem", "identity.pem", "identity-key.pem"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("invalid-test-fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := Load(
		filepath.Join(directory, "ca.pem"),
		filepath.Join(directory, "identity.pem"),
		filepath.Join(directory, "identity-key.pem"),
	)
	if !errors.Is(err, ErrInsecurePrivateKey) {
		t.Fatalf("Load() error = %v, want ErrInsecurePrivateKey", err)
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
