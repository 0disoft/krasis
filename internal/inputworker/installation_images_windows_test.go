//go:build windows

package inputworker

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// Ordinary temporary files only: no installation, ACL or real input changes.
func installationImagesFixture(t testing.TB, size int) (string, [2][sha256.Size]byte) {
	t.Helper()
	directory := t.TempDir()
	var digests [2][sha256.Size]byte
	for i, name := range [2]string{"krasis-agent.exe", "krasis-input.exe"} {
		data := make([]byte, size)
		data[0] = byte(i + 1)
		digests[i] = sha256.Sum256(data)
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return directory, digests
}

func TestInstallationImagesPinsExcludeWritesUntilClosed(t *testing.T) {
	directory, digests := installationImagesFixture(t, 64)
	images, err := lockInstallationImages(directory, digests)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, image := range images {
			_ = image.file.Close()
		}
	})
	for i, image := range images {
		if image.digest != digests[i] || image.file == nil {
			t.Fatal("image order or digest changed")
		}
		if os.WriteFile(image.path, []byte("replacement"), 0600) == nil || os.Rename(image.path, image.path+".moved") == nil {
			t.Fatal("verification released a live image pin")
		}
		if err := image.file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(image.path, []byte("replacement"), 0600); err != nil {
			t.Fatal("closed pin leaked", err)
		}
	}
}

func TestInstallationImagesFailureJoinsAndClosesBothPins(t *testing.T) {
	for _, badIndex := range []int{0, 1} {
		t.Run([2]string{"agent", "input"}[badIndex], func(t *testing.T) {
			directory, digests := installationImagesFixture(t, 64)
			digests[badIndex][0] ^= 1
			images, err := lockInstallationImages(directory, digests)
			if err != ErrAuthority || images[0].file != nil || images[1].file != nil {
				t.Fatal("partial installation escaped verification")
			}
			for _, name := range [2]string{"krasis-agent.exe", "krasis-input.exe"} {
				path := filepath.Join(directory, name)
				if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
					t.Fatal("failed verification leaked a pin", err)
				}
				if err := os.Rename(path, path+".moved"); err != nil {
					t.Fatal("failed verification leaked delete exclusion", err)
				}
			}
		})
	}
}

func TestInstallationImagesMissingFileClosesVerifiedSibling(t *testing.T) {
	directory, digests := installationImagesFixture(t, 64)
	if err := os.Remove(filepath.Join(directory, "krasis-input.exe")); err != nil {
		t.Fatal(err)
	}
	if images, err := lockInstallationImages(directory, digests); err != ErrAuthority || images[0].file != nil || images[1].file != nil {
		t.Fatal("incomplete installation escaped verification")
	}
	if err := os.WriteFile(filepath.Join(directory, "krasis-agent.exe"), []byte("replacement"), 0600); err != nil {
		t.Fatal("missing file left sibling pinned", err)
	}
}

func BenchmarkInstallationImages(b *testing.B) {
	directory, digests := installationImagesFixture(b, 8<<20)
	for _, parallel := range []bool{false, true} {
		name := "serial"
		if parallel {
			name = "parallel"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var images [2]installationImage
				if parallel {
					var err error
					images, err = lockInstallationImages(directory, digests)
					if err != nil {
						b.Fatal(err)
					}
				} else {
					for i, name := range [2]string{"krasis-agent.exe", "krasis-input.exe"} {
						file, err := lockImage(filepath.Join(directory, name), digests[i])
						if err != nil {
							b.Fatal(err)
						}
						images[i].file = file
					}
				}
				for _, image := range images {
					if err := image.file.Close(); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
