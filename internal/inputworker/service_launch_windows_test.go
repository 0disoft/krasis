//go:build windows

package inputworker

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestUnboundServiceLaunchPinChecksLiveOwnershipAndInstallation(t *testing.T) {
	// Read-only current-process handle and ordinary temp image; never a service,
	// child process, elevated operation or input injection.
	image, err := os.Create(filepath.Join(t.TempDir(), "image"))
	if err != nil {
		t.Fatal(err)
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, windows.GetCurrentProcessId())
	if err != nil {
		image.Close()
		t.Fatal(err)
	}
	digest := [32]byte{2}
	installation := &Installation{files: make([]*os.File, 5), agentDigest: [32]byte{1}, serviceDigest: digest}
	owner := &serviceLaunchOwner{installation: installation, digest: digest, process: process, image: image}
	defer owner.Close()
	for range 6 {
		if !owner.check() {
			t.Fatal("live exact launch pin rejected")
		}
	}
	installation.serviceDigest = [32]byte{3}
	if owner.check() {
		t.Fatal("different installation admitted unbound creation")
	}
	installation.serviceDigest = digest
	installation.closed = true
	if owner.check() {
		t.Fatal("closed installation admitted unbound creation")
	}
	installation.closed = false
	if owner.Close() != nil || owner.check() || owner.Close() != nil {
		t.Fatal("closed launch owner revived or retried its handles")
	}
}

func TestUnboundServiceLaunchRejectsOrdinaryUninstalledOwner(t *testing.T) {
	if owner, err := pinServiceLaunchOwner(nil, [32]byte{}); owner != nil || err == nil {
		t.Fatal("missing protected SYSTEM installation admitted launch")
	}
}
