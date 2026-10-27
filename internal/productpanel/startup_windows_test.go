//go:build windows

package productpanel

import (
	"crypto/sha256"
	"errors"
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

func TestStartupRejectsOversizedBundleFiles(t *testing.T) {
	for _, name := range []string{"SHA256SUMS.txt", "krasis-agent.exe"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			manifest := ""
			for _, image := range []string{"krasis-control.exe", "krasis-agent.exe"} {
				if err := os.WriteFile(filepath.Join(root, image), []byte(image), 0600); err != nil {
					t.Fatal(err)
				}
				manifest += fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(image)), image)
			}
			if err := os.WriteFile(filepath.Join(root, "SHA256SUMS.txt"), []byte(manifest), 0600); err != nil {
				t.Fatal(err)
			}
			limit := maxStartupImageBytes
			if name == "SHA256SUMS.txt" {
				limit = 8192
			}
			if err := os.Truncate(filepath.Join(root, name), limit+1); err != nil {
				t.Fatal(err)
			}
			if err := verifyAgentBundle(root, "krasis-control.exe"); !errors.Is(err, errStartupBundleFile) {
				t.Fatal("oversized bundle file was not rejected", err)
			}
		})
	}
}

func BenchmarkStartupBundleVerification(b *testing.B) {
	root := b.TempDir()
	data := make([]byte, 8<<20)
	digest := sha256.Sum256(data)
	manifest := ""
	for _, name := range []string{"krasis-control.exe", "krasis-agent.exe"} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			b.Fatal(err)
		}
		manifest += fmt.Sprintf("%x  %s\n", digest, name)
	}
	if err := os.WriteFile(filepath.Join(root, "SHA256SUMS.txt"), []byte(manifest), 0600); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(2 * len(data)))
	b.ResetTimer()
	for b.Loop() {
		if err := verifyAgentBundle(root, "krasis-control.exe"); err != nil {
			b.Fatal(err)
		}
	}
}
