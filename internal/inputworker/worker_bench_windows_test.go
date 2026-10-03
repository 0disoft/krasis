//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"os"
	"runtime"
	"testing"
	"time"
)

// Measures the real native pipe and locked-thread request/ACK path with a fake
// input adapter. It never injects input or starts the installed service.
func BenchmarkNativeWorkerRoundtrip(b *testing.B) {
	server, conn := nativePair(b)
	lease := testLease()
	done := make(chan finish, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		report, err := serve(context.Background(), server, &recordingInput{}, lease, testExecute, testExecute, time.Now)
		done <- finish{report, err}
	}()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = client.Close() })
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := client.Move(context.Background(), 1, 1); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = client.Close()
	select {
	case result := <-done:
		if result.err != nil || result.report.Completed != uint64(b.N) {
			b.Fatal("roundtrip completion mismatch")
		}
	case <-time.After(time.Second):
		b.Fatal("worker cleanup stalled")
	}
}

func BenchmarkInstalledImagePin(b *testing.B) {
	installation := imagePinFixture(b)
	image := installation.images[1]
	if err := image.file.Close(); err != nil {
		b.Fatal(err)
	}
	data := make([]byte, 8<<20)
	if err := os.WriteFile(image.path, data, 0600); err != nil {
		b.Fatal(err)
	}
	digest := sha256.Sum256(data)
	file, err := lockImage(image.path, digest)
	if err != nil {
		b.Fatal(err)
	}
	installation.files[4] = file
	installation.images[1] = installationImage{path: image.path, digest: digest, file: file}
	for _, name := range []string{"rehash", "held-file"} {
		b.Run(name, func(b *testing.B) {
			var proof *Installation
			if name == "held-file" {
				proof = installation
			}
			b.ReportAllocs()
			for b.Loop() {
				pin, err := pinIdentityImage(proof, image.path, digest)
				if err != nil {
					b.Fatal(err)
				}
				if err := pin.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
