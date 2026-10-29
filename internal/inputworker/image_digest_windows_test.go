//go:build windows

package inputworker

import (
	"bytes"
	"crypto/sha256"
	"runtime"
	"testing"

	"golang.org/x/sys/cpu"
)

func TestWindowsImageDigestMatchesWholeSHA256(t *testing.T) {
	for _, n := range []int{1, 55, 64, 65, imageHashReadSize + 1, 9 << 20} {
		data := bytes.Repeat([]byte{37}, n)
		var timing imageHashTimings
		if err := verifyImageHashUsing(bytes.NewReader(data), int64(n), sha256.Sum256(data), &timing, newWindowsImageDigest); err != nil || timing.backend != 2 {
			t.Fatal("native SHA256 differs from full image digest", n, err, timing)
		}
		if err := verifyImageHashUsing(bytes.NewReader(data), int64(n), sha256.Sum256([]byte("changed")), nil, newWindowsImageDigest); err != ErrAuthority {
			t.Fatal("native hashing accepted corruption", n, err)
		}
		if err := verifyImageHashUsing(bytes.NewReader(data[:n-1]), int64(n), sha256.Sum256(data), nil, newWindowsImageDigest); err != ErrAuthority {
			t.Fatal("native hashing accepted truncation", n, err)
		}
		if err := verifyImageHashUsing(bytes.NewReader(append(data, 1)), int64(n), sha256.Sum256(data), nil, newWindowsImageDigest); err != ErrAuthority {
			t.Fatal("native hashing accepted growth", n, err)
		}
	}
	got, err := newImageDigest()
	if err != nil {
		t.Fatal(err)
	}
	defer got.close()
	want := uint8(1)
	if runtime.GOARCH == "amd64" && !cpu.X86.HasAVX {
		want = 2
	}
	if got.backend != want {
		t.Fatal("wrong CPU hashing path", got.backend, want)
	}
}

// GODEBUG=cpu.all=off exercises the Go generic fallback without changing
// Windows' own feature selection. This is not emulation of another laptop.
func BenchmarkImageDigest(b *testing.B) {
	data := bytes.Repeat([]byte{37}, 9<<20)
	want := sha256.Sum256(data)
	for _, tc := range []struct {
		name   string
		create func() (*imageDigest, error)
	}{{"go", newGoImageDigest}, {"windows", newWindowsImageDigest}} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if err := verifyImageHashUsing(bytes.NewReader(data), int64(len(data)), want, nil, tc.create); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
