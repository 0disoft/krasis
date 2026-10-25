//go:build windows

package inputworker

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Model the installed input image's roughly 9 MB full hash, including a fresh
// buffer for each pin. This measures local cached reads, not B's startup time.
func BenchmarkImageHashReadSize(b *testing.B) {
	path := filepath.Join(b.TempDir(), "image")
	data := make([]byte, 9<<20)
	if err := os.WriteFile(path, data, 0600); err != nil {
		b.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	for _, size := range []int{32 << 10, 256 << 10, 1 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					b.Fatal(err)
				}
				hash := sha256.New()
				if n, err := io.CopyBuffer(hash, io.LimitReader(file, int64(len(data))+1), make([]byte, size)); err != nil || n != int64(len(data)) {
					b.Fatal("full image hash failed", n, err)
				}
			}
		})
	}
}
