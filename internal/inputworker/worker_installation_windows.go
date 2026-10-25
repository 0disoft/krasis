//go:build windows

package inputworker

import (
	"crypto/sha256"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Only worker startup may read these digests while the agent pin is pending.
// digests() still rejects this state for the service and all other callers.
func (installation *Installation) workerDigests() ([sha256.Size]byte, [sha256.Size]byte, error) {
	if installation == nil {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrAuthority
	}
	installation.mu.Lock()
	defer installation.mu.Unlock()
	if installation.closed || !installation.workerPending || len(installation.files) != 4 || installation.images[0].file != nil || installation.images[1].file == nil || installation.agentDigest == ([sha256.Size]byte{}) || installation.serviceDigest == ([sha256.Size]byte{}) {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrAuthority
	}
	return installation.agentDigest, installation.serviceDigest, nil
}

// The caller owns file on failure; ownership transfers only on success. Never
// accept a handle before independent service-image, pipe-owner and SCM checks.
func (installation *Installation) adoptWorkerAgentImage(proof *Authority, file *os.File) error {
	return installation.adoptWorkerAgentImageChecked(proof, file, func(path string) bool {
		fixed, err := openProtected(path, false)
		if err != nil {
			return false
		}
		matched := samePinnedImage(file, fixed) && protectedHandle(windows.Handle(file.Fd())) == nil
		return fixed.Close() == nil && matched
	})
}

// Verification grants no authority and transfers no ownership. Check the live
// service once, AFTER exact file/ACL validation and immediately before commit.
// A service exit/restart during verification must reject adoption; an earlier
// SCM query cannot replace this final check and only repeats the same RPC.
func (installation *Installation) adoptWorkerAgentImageChecked(proof *Authority, file *os.File, verify func(string) bool) error {
	if installation == nil || file == nil || proof == nil || proof.kind != authorityService || verify == nil {
		return ErrAuthority
	}
	installation.mu.Lock()
	defer installation.mu.Unlock()
	if installation.closed || !installation.workerPending || len(installation.files) != 4 || installation.images[0].file != nil || installation.images[1].file == nil || installation.agentDigest == ([sha256.Size]byte{}) {
		return ErrAuthority
	}
	path := filepath.Join(filepath.Dir(installation.images[1].path), "krasis-agent.exe")
	// Reopening the protected fixed name only checks exact file identity. No
	// path/digest cache authorizes bytes; the authenticated service owns the
	// original full-hash pin and DuplicateHandle preserves that file object.
	if !verify(path) || proof.check(false) != nil {
		return ErrAuthority
	}
	installation.images[0] = installationImage{path: path, digest: installation.agentDigest, file: file}
	installation.files = append(installation.files, file)
	installation.workerPending = false
	return nil
}

func samePinnedImage(file, fixed *os.File) bool {
	if file == nil || fixed == nil {
		return false
	}
	var actual, expected windows.ByHandleFileInformation
	for i, item := range []*os.File{file, fixed} {
		handle := windows.Handle(item.Fd())
		kind, err := windows.GetFileType(handle)
		info := &actual
		if i == 1 {
			info = &expected
		}
		if err != nil || kind != windows.FILE_TYPE_DISK || windows.GetFileInformationByHandle(handle, info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
			return false
		}
		size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
		if size == 0 || size > 64<<20 {
			return false
		}
	}
	return actual.VolumeSerialNumber == expected.VolumeSerialNumber && actual.FileIndexHigh == expected.FileIndexHigh && actual.FileIndexLow == expected.FileIndexLow && actual.FileSizeHigh == expected.FileSizeHigh && actual.FileSizeLow == expected.FileSizeLow
}
