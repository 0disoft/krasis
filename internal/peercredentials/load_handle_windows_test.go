//go:build windows

package peercredentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPrivateKeyKeepsOpenedFileAcrossPathReplacement(t *testing.T) {
	dir := t.TempDir()
	if _, err := ProtectSecurityDirectory(dir); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "identity-key.pem")
	replacement := filepath.Join(dir, "replacement.pem")
	for name, contents := range map[string]string{key: "original", replacement: "replacement"} {
		if err := os.WriteFile(name, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ProtectSecurityFile(name); err != nil {
			t.Fatal(err)
		}
	}
	var swapErr error
	data, err := readPrivateKey(key, func(stage string) {
		if stage != "wrap" {
			return
		}
		swapErr = os.Rename(key, filepath.Join(dir, "old.pem"))
		if swapErr == nil {
			swapErr = os.Rename(replacement, key)
		}
	})
	if swapErr != nil {
		t.Fatal(swapErr)
	}
	if err != nil || string(data) != "original" {
		t.Fatalf("readPrivateKey = %q, %v; want original opened file", data, err)
	}
}
