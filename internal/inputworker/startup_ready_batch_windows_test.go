//go:build windows

package inputworker

import (
	"testing"
	"time"
)

func BenchmarkStartupReadyNativePipe(b *testing.B) {
	packet := startupBatchPacket(b)
	server, client := nativePair(b)
	if err := server.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		b.Fatal(err)
	}
	if err := client.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		b.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, err := server.Write(packet); err != nil {
				return
			}
		}
	}()
	defer func() {
		_ = server.Close()
		<-done
	}()
	reader := &startupCountingReader{Reader: client}
	b.ReportAllocs()
	for b.Loop() {
		reader.calls = 0
		if p, err := readInputLeaseReady(reader); err != nil || p == nil {
			b.Fatal(p, err)
		}
		b.ReportMetric(float64(reader.calls), "reads/op")
	}
}
