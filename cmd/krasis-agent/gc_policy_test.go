package main

import (
	"os"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestResidentGCDefaultsAndOperatorOverrides(t *testing.T) {
	for _, env := range []string{"unset", "0", "100", "off", ""} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("GOGC", env)
			if env == "unset" {
				if err := os.Unsetenv("GOGC"); err != nil {
					t.Fatal(err)
				}
			}
			previous := debug.SetGCPercent(100)
			defer debug.SetGCPercent(previous)
			limit := debug.SetMemoryLimit(-1)
			configureResidentGC()
			actual := debug.SetGCPercent(100)
			want := 100
			if env == "unset" {
				want = residentGCPercent
			}
			if actual != want || debug.SetMemoryLimit(-1) != limit {
				t.Fatal("GC default or operator memory policy changed", actual, want)
			}
		})
	}
}

var residentGCBenchmarkSink []byte

// A fixed small live heap and equal allocation volume isolate GC pacing, not
// Windows scheduling or physical input latency. Run with -benchtime=32768x.
func BenchmarkResidentGCBudget(b *testing.B) {
	for _, percent := range []int{100, residentGCPercent} {
		name := "default"
		if percent == residentGCPercent {
			name = "resident"
		}
		b.Run(name, func(b *testing.B) {
			previous := debug.SetGCPercent(percent)
			defer debug.SetGCPercent(previous)
			live := make([]byte, 2<<20)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			b.SetBytes(4096)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				residentGCBenchmarkSink = make([]byte, 4096)
			}
			b.StopTimer()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(live)
			b.ReportMetric(float64(after.NumGC-before.NumGC), "gc-cycles")
			b.ReportMetric(float64(after.HeapAlloc)/(1<<20), "final-heap-MiB")
		})
	}
}
