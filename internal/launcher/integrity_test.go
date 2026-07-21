package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyKitAcceptsPackagedFilesAndIgnoresRuntimeData(t *testing.T) {
	base := t.TempDir()
	writeIntegrityFixture(t, base)
	if err := os.Mkdir(filepath.Join(base, "reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "launcher-config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyKit(base); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyKitRejectsTamperedExecutable(t *testing.T) {
	base := t.TempDir()
	writeIntegrityFixture(t, base)
	if err := os.WriteFile(filepath.Join(base, "krasis-lab.exe"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyKit(base); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("VerifyKit tamper error = %v", err)
	}
}

func writeIntegrityFixture(t *testing.T, base string) {
	t.Helper()
	var lines []string
	for _, name := range kitFiles {
		payload := []byte("fixture:" + name)
		if err := os.WriteFile(filepath.Join(base, name), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(payload)
		lines = append(lines, hex.EncodeToString(sum[:])+"  "+name)
	}
	if err := os.WriteFile(filepath.Join(base, "SHA256SUMS.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
