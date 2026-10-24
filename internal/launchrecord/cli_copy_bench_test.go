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
