//go:build windows

package inputworker

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Unit fixture for native pin lifetime, not a protected installation or a
// service authorization. No ACL, service, privilege or real input is changed.
func imagePinFixture(t testing.TB) *Installation {
	t.Helper()
	installation := &Installation{}
	root := t.TempDir()
	t.Cleanup(func() { _ = installation.Close() })
	for i, name := range []string{"root-pin", "directory-pin", "metadata-pin", "agent-image", "service-image"} {
		path := filepath.Join(root, name)
		data := []byte{byte(i), 1, 2, 3}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		pin, err := lockImage(path, digest)
		if err != nil {
			t.Fatal("fixture image pin failed")
		}
		installation.files = append(installation.files, pin)
		if i >= 3 {
			installation.images[i-3] = installationImage{path: path, digest: digest, file: pin}
		}
	}
	return installation
}

func TestInstallationImageDuplicatePreservesFileIdentityAndWriteLock(t *testing.T) {
	installation := imagePinFixture(t)
	image := installation.images[0]
	duplicate, err := installation.duplicateImage(image.path, image.digest)
	if err != nil {
		t.Fatal("verified image duplicate failed")
	}
	t.Cleanup(func() { _ = duplicate.Close() })
	var before, after windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(image.file.Fd()), &before) != nil || windows.GetFileInformationByHandle(windows.Handle(duplicate.Fd()), &after) != nil || before.VolumeSerialNumber != after.VolumeSerialNumber || before.FileIndexHigh != after.FileIndexHigh || before.FileIndexLow != after.FileIndexLow {
		t.Fatal("duplicate does not pin the original file object")
	}
	if err := installation.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image.path, []byte("replacement"), 0600); err == nil {
		t.Fatal("original close released the duplicate's write exclusion")
	}
	if err := os.Rename(image.path, image.path+".moved"); err == nil {
		t.Fatal("original close released the duplicate's delete exclusion")
	}
	if err := duplicate.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image.path, []byte("replacement"), 0600); err != nil {
		t.Fatal("image pin leaked after final owner closed")
	}
}

func TestInstallationImagePinRejectsWrongPathDigestAndClosedProof(t *testing.T) {
	installation := imagePinFixture(t)
	image := installation.images[0]
	for _, request := range []struct {
		path   string
		digest [32]byte
	}{
		{image.path, [32]byte{}},
		{image.path, installation.images[1].digest},
		{installation.images[1].path, image.digest},
		{image.path + ".other", image.digest},
	} {
		if pin, err := pinIdentityImage(installation, request.path, request.digest); pin != nil || err != ErrAuthority {
			if pin != nil {
				_ = pin.Close()
			}
			t.Fatal("unmatched pin accepted")
		}
	}
	if err := installation.Close(); err != nil {
		t.Fatal(err)
	}
	// The pathname still contains matching bytes. Reopening it would conceal
	// the closed authority and extend a retired installation's trust lifetime.
	if pin, err := pinIdentityImage(installation, image.path, image.digest); pin != nil || err != ErrAuthority {
		if pin != nil {
			_ = pin.Close()
		}
		t.Fatal("closed installation reopened its former image")
	}
	var zero Installation
	if pin, err := zero.duplicateImage(image.path, image.digest); pin != nil || err != ErrAuthority {
		t.Fatal("zero installation supplied image authority")
	}
}

func TestClosingImageProofDoesNotReleaseInstallationPin(t *testing.T) {
	installation := imagePinFixture(t)
	image := installation.images[1]
	proof, err := installation.duplicateImage(image.path, image.digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := proof.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image.path, []byte("replacement"), 0600); err == nil {
		t.Fatal("proof close released the installation pin")
	}
	if pin, err := installation.duplicateImage(image.path, image.digest); err != nil {
		t.Fatal("proof close damaged the installation's handle")
	} else {
		_ = pin.Close()
	}
}
