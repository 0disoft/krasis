//go:build windows

package inputworker

import (
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

// Read-only checks on the current protected installation's pinned objects.
// No inputs, ACL writes, service control or installation changes. The fixture
// loading is outside the timed loop; these numbers isolate the ACL queries.
func BenchmarkInstalledImageACL(b *testing.B) {
	installation, err := LoadInstallation()
	if err != nil {
		b.Skip("protected installation unavailable")
	}
	defer installation.Close()
	for _, parallel := range []bool{false, true} {
		name := "serial"
		if parallel {
			name = "parallel"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var failures [2]error
				if parallel {
					var joined sync.WaitGroup
					for i, image := range installation.images {
						joined.Go(func() { failures[i] = protectedHandle(windows.Handle(image.file.Fd())) })
					}
					joined.Wait()
				} else {
					for i, image := range installation.images {
						failures[i] = protectedHandle(windows.Handle(image.file.Fd()))
					}
				}
				if failures[0] != nil || failures[1] != nil {
					b.Fatal("protected image query failed")
				}
			}
		})
	}
}
