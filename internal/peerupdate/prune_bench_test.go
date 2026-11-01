package peerupdate

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkPlanPruneFolders(b *testing.B) {
	for _, count := range []int{16, 1024} {
		for _, protected := range []int{2, 64} {
			for _, algorithm := range []struct {
				name string
				plan func([]pruneFolder, []string, string, string, time.Time) []string
			}{{"linear", linearPlanPruneFolders}, {"indexed", planPruneFolders}} {
				b.Run(fmt.Sprintf("folders=%d/protected=%d/%s", count, protected, algorithm.name), func(b *testing.B) {
					now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
					base := filepath.Join("Users", "operator", "AppData", "Local", "Krasis", "updates")
					folders := make([]pruneFolder, count)
					for i := range folders {
						folders[i] = pruneFolder{fmt.Sprintf("%032x", i), now.Add(-48 * time.Hour)}
					}
					keep := make([]string, protected)
					for i := range keep {
						keep[i] = filepath.Join(base, fmt.Sprintf("%032x", count-1+i))
					}
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						if got := algorithm.plan(folders, keep, "", base, now); len(got) != count-3 {
							b.Fatalf("planned %d removals, want %d", len(got), count-3)
						}
					}
				})
			}
		}
	}
}
