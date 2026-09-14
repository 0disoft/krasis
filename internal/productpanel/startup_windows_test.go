//go:build windows

package productpanel

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupRejectsIncompleteOrTamperedBundle(t *testing.T) {
	root := t.TempDir()
	manifest := ""
	for _, name := range []string{"krasis-control.exe", "krasis-agent.exe"} {
		data := []byte(name)
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		manifest += fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name)
	}
	if verifyAgentBundle(root, "krasis-control.exe") == nil {
		t.Fatal("missing manifest accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "SHA256SUMS.txt"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyAgentBundle(root, "krasis-control.exe"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "krasis-agent.exe"), []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyAgentBundle(root, "krasis-control.exe") == nil {
		t.Fatal("modified binary accepted")
	}
}
