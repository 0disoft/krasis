//go:build linux

package peercredentials_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
)

func TestLinuxLocalAPICredentialsRejectPermissionChanges(t *testing.T) {
	base := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(base); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "control")
	session, err := localapi.PublishCredentials(path, "127.0.0.1:12345")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	credentials, err := localapi.LoadClientCredentials(path)
	if err != nil || credentials.Token != session.Token() {
		t.Fatalf("same-user credentials = %v", err)
	}
	if err := os.Chmod(filepath.Join(path, "auth-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := localapi.LoadClientCredentials(path); err == nil {
		t.Fatal("world-readable token accepted")
	}
	if err := os.Chmod(filepath.Join(path, "auth-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := localapi.LoadClientCredentials(path); err == nil {
		t.Fatal("non-private control directory accepted")
	}
}
