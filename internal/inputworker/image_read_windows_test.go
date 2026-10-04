//go:build windows

package inputworker

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestImageVerificationIncludesFinalPartialReadAndReleasesRejectedPin(t *testing.T) {
	// Cross several full reads and a partial tail. A tail-only mutation must
	// fail the original digest, even with a larger verification read buffer.
	data := make([]byte, (512<<10)+17)
	data[0], data[len(data)-1] = 1, 2
	path := filepath.Join(t.TempDir(), "image.exe")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	file, err := lockImage(path, digest)
	if err != nil {
		t.Fatal("full image was not verified", err)
	}
	if os.WriteFile(path, data, 0600) == nil {
		t.Fatal("live verification shared writes")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if file, err := lockImage(path, digest); file != nil || err != ErrAuthority {
		if file != nil {
			_ = file.Close()
		}
		t.Fatal("tail-only image mutation was accepted")
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal("rejected digest leaked delete exclusion", err)
	}
}

func TestImageVerificationRejectsEmptyAndOversizedFilesBeforeReading(t *testing.T) {
	for _, size := range []int64{0, (64 << 20) + 1} {
		path := filepath.Join(t.TempDir(), "image.exe")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(size); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte("different image"))
		if locked, err := lockImage(path, digest); locked != nil || err != ErrAuthority {
			if locked != nil {
				_ = locked.Close()
			}
			t.Fatal("invalid image size was accepted", size)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal("invalid size retained a pin", err)
		}
	}
}
