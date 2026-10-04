//go:build windows

package inputworker

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Warm temporary-image scan only. No installed files, input, policy or service
// state is changed. This isolates read/hash cost, not whole-worker readiness.
func BenchmarkImageRead(b *testing.B) {
	directory, digests := installationImagesFixture(b, 8<<20)
	name, err := windows.UTF16PtrFromString(filepath.Join(directory, "krasis-agent.exe"))
	if err != nil {
		b.Fatal(err)
	}
	for _, sequential := range []bool{false, true} {
		for _, size := range []int{32 << 10, 128 << 10, 256 << 10} {
			b.Run(fmt.Sprintf("sequential=%t/buffer=%d", sequential, size), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(8 << 20)
				for b.Loop() {
					flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
					if sequential {
						flags |= windows.FILE_FLAG_SEQUENTIAL_SCAN
					}
					handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
					if err != nil {
						b.Fatal(err)
					}
					file := os.NewFile(uintptr(handle), "benchmark-image")
					hash := sha256.New()
					n, readErr := io.CopyBuffer(hash, io.LimitReader(file, (8<<20)+1), make([]byte, size))
					closeErr := file.Close()
					var digest [sha256.Size]byte
					copy(digest[:], hash.Sum(nil))
					if n != 8<<20 || readErr != nil || closeErr != nil || digest != digests[0] {
						b.Fatal("incomplete or incorrect image scan", readErr, closeErr)
					}
				}
			})
		}
	}
}
