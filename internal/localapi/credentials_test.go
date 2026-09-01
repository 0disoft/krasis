package localapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
)

func TestCredentialSessionPublishesLoadsAndRemovesProtectedFiles(t *testing.T) {
	root := protectedRoot(t)
	directory := filepath.Join(root, "control")
	session, err := PublishCredentials(directory, "127.0.0.1:24877")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := LoadClientCredentials(directory)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Endpoint.Address != "127.0.0.1:24877" || credentials.Token != session.Token() || len(credentials.Token) != 64 {
		t.Fatalf("credentials = %+v", credentials)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{endpointFile, tokenFile} {
		if _, err := os.Lstat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatalf("%s remains after Close(): %v", name, err)
		}
	}
}

func TestCredentialStoreRejectsNonLoopbackAndInsecureDirectory(t *testing.T) {
	root := protectedRoot(t)
	if _, err := PublishCredentials(filepath.Join(root, "control"), "0.0.0.0:24877"); err == nil {
		t.Fatal("PublishCredentials() accepted a non-loopback address")
	}
	directory := filepath.Join(root, "insecure")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishCredentials(directory, "127.0.0.1:24877"); err == nil {
		t.Fatal("PublishCredentials() accepted an insecure directory")
	}
}

func TestClientCredentialsRejectUnknownAndTrailingEndpointJSON(t *testing.T) {
	root := protectedRoot(t)
	directory := filepath.Join(root, "control")
	session, err := PublishCredentials(directory, "127.0.0.1:24877")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	path := filepath.Join(directory, endpointFile)
	for _, payload := range []string{
		`{"schemaVersion":"krasis.local-api-endpoint/v1","address":"127.0.0.1:24877","unknown":true}`,
		`{"schemaVersion":"krasis.local-api-endpoint/v1","address":"127.0.0.1:24877"} {}`,
	} {
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadClientCredentials(directory); err == nil {
			t.Fatalf("LoadClientCredentials() accepted %q", payload)
		}
	}
}

func protectedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(root); err != nil {
		t.Fatal(err)
	}
	return root
}
