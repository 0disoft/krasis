package labcli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePeerCredentialsArgs(t *testing.T) {
	options, err := parsePeerCredentialsArgs([]string{"--output", "secrets", "--server-name=peer.test"})
	if err != nil {
		t.Fatalf("parsePeerCredentialsArgs() error = %v", err)
	}
	if options.outputDirectory != "secrets" || options.serverName != "peer.test" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParsePeerCredentialsArgsRequiresBothValues(t *testing.T) {
	if _, err := parsePeerCredentialsArgs([]string{"--output", "secrets"}); err == nil {
		t.Fatal("parsePeerCredentialsArgs() accepted missing server name")
	}
}

func TestParsePeerCredentialsProtectArgs(t *testing.T) {
	options, err := parsePeerCredentialsProtectArgs([]string{"--directory=listener"})
	if err != nil {
		t.Fatalf("parsePeerCredentialsProtectArgs() error = %v", err)
	}
	if options.directory != "listener" {
		t.Fatalf("directory = %q", options.directory)
	}
}

func TestRunPeerCredentialsProtectSecuresCopiedRoleDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "listener")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "identity-key.pem"), []byte("test-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := runPeerCredentialsProtect([]string{"--directory", directory}, &stdout, &stderr); exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if stdout.String() == "" {
		t.Fatal("expected success output")
	}
}
