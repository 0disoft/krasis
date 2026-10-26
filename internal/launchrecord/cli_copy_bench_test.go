package launchrecord

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/productidentity"
)

func BenchmarkCopyUnchangedCLI8MiB(b *testing.B) {
	source := b.TempDir()
	data := make([]byte, 8<<20)
	copy(data, "MZ")
	if err := os.WriteFile(filepath.Join(source, "krasis-cli.exe"), data, 0600); err != nil {
		b.Fatal(err)
	}
	destination, err := productidentity.PrepareLaunch(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer destination.Close()
	if err := replace(destination, "krasis-cli.exe", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := copyCLI(destination, source); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCopyNewCLI(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int64
	}{{"8MiB", 8 << 20}, {"64MiB", maxExecutable}} {
		b.Run(tc.name, func(b *testing.B) {
			source := b.TempDir()
			file, err := os.Create(filepath.Join(source, "krasis-cli.exe"))
			if err != nil {
				b.Fatal(err)
			}
			truncateErr := file.Truncate(tc.size)
			closeErr := file.Close()
			if truncateErr != nil || closeErr != nil {
				b.Fatal(truncateErr, closeErr)
			}
			destination, err := productidentity.PrepareLaunch(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer destination.Close()
			b.ReportAllocs()
			b.SetBytes(tc.size)
			for b.Loop() {
				if err := copyCLI(destination, source); err != nil {
					b.Fatal(err)
				}
				if err := destination.Remove("krasis-cli.exe"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
