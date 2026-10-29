//go:build windows

package inputworker

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// This proof is only for creating a child with no agent handles, lease or input.
// Pin the exact verified SYSTEM service/image once for this launch transaction;
// checkpoints still check live ownership and installation invalidation. Actual
// child inspection and independent service/agent bootstrap remain mandatory.
type serviceLaunchOwner struct {
	installation *Installation
	digest       [sha256.Size]byte
	process      windows.Handle
	image        *os.File
	closed       bool
	closeErr     error
}

func pinServiceLaunchOwner(installation *Installation, digest [sha256.Size]byte) (*serviceLaunchOwner, error) {
	if installation == nil || !fixedInstalledInputProcess(digest, false, installation) {
		return nil, ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	image, err := pinIdentityImage(installation, filepath.Join(root, "Krasis", "Input", "krasis-input.exe"), digest)
	if err != nil {
		return nil, err
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, windows.GetCurrentProcessId())
	if err != nil {
		return nil, errors.Join(ErrAuthority, err, image.Close())
	}
	return &serviceLaunchOwner{installation: installation, digest: digest, process: process, image: image}, nil
}

func (p *serviceLaunchOwner) check() bool {
	if p == nil || p.closed || p.image == nil || p.process == 0 {
		return false
	}
	_, service, err := p.installation.digests()
	return err == nil && service == p.digest && nativeParentAlive(p.process)
}

func (p *serviceLaunchOwner) Close() error {
	if p == nil {
		return nil
	}
	if p.closed {
		return p.closeErr
	}
	p.closed = true
	p.closeErr = errors.Join(p.image.Close(), windows.CloseHandle(p.process))
	return p.closeErr
}
