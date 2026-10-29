//go:build windows

package inputworker

import (
	"crypto/sha256"
	"runtime"
	"unsafe"

	"golang.org/x/sys/cpu"
	"golang.org/x/sys/windows"
)

var imageCrypto = windows.NewLazySystemDLL("bcrypt.dll")
var imageCreateHash = imageCrypto.NewProc("BCryptCreateHash")
var imageHashData = imageCrypto.NewProc("BCryptHashData")
var imageFinishHash = imageCrypto.NewProc("BCryptFinishHash")
var imageDestroyHash = imageCrypto.NewProc("BCryptDestroyHash")

func newImageDigest() (*imageDigest, error) {
	// Go's amd64 SHA-NI and AVX2 paths both require AVX. On non-AVX
	// machines CNG can use Windows' native implementation instead of Go's
	// generic block loop. Other CPUs keep the existing accelerated Go path.
	if runtime.GOARCH == "amd64" && !cpu.X86.HasAVX {
		return newWindowsImageDigest()
	}
	return newGoImageDigest()
}

func newWindowsImageDigest() (*imageDigest, error) {
	for _, proc := range []*windows.LazyProc{imageCreateHash, imageHashData, imageFinishHash, imageDestroyHash} {
		if proc.Find() != nil {
			return nil, ErrAuthority
		}
	}
	var handle uintptr
	// Fixed Windows 10+ BCRYPT_SHA256_ALG_HANDLE; no caller-selected provider,
	// key, algorithm or reusable digest state. CNG owns the hash object memory.
	status, _, _ := imageCreateHash.Call(0x41, uintptr(unsafe.Pointer(&handle)), 0, 0, 0, 0, 0)
	if status != 0 || handle == 0 {
		return nil, ErrAuthority
	}
	return &imageDigest{backend: 2,
		write: func(b []byte) error {
			if len(b) == 0 {
				return nil
			}
			status, _, _ := imageHashData.Call(handle, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
			runtime.KeepAlive(b)
			if status != 0 {
				return ErrAuthority
			}
			return nil
		},
		finish: func() ([sha256.Size]byte, error) {
			var sum [sha256.Size]byte
			status, _, _ := imageFinishHash.Call(handle, uintptr(unsafe.Pointer(&sum[0])), uintptr(len(sum)), 0)
			if status != 0 {
				return sum, ErrAuthority
			}
			return sum, nil
		},
		close: func() error {
			status, _, _ := imageDestroyHash.Call(handle)
			if status != 0 {
				return ErrAuthority
			}
			return nil
		},
	}, nil
}
