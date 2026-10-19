//go:build windows

package inputworker

import (
	"io"
	"runtime"
	"testing"
)

// Pipe transport only: no input is injected and no lease is authorized. The
// worker side owns one OS thread as in the product's serialized input loop.
func BenchmarkNativePipeRoundTrip(b *testing.B) {
	server, client := nativePair(b)
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var frame [32]byte
		for range b.N {
			if _, err := io.ReadFull(server, frame[:]); err != nil {
				done <- err
				return
			}
			if _, err := server.Write(frame[:]); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	var frame [32]byte
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := client.Write(frame[:]); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(client, frame[:]); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if err := <-done; err != nil {
		b.Fatal(err)
	}
}
