//go:build windows

package inputworker

import (
	"io"
	"testing"
	"time"
)

func TestNativeRetirementReceiptSurvivesImmediateWriterClose(t *testing.T) {
	for range 20 {
		server, client := nativePair(t)
		written := make(chan error, 1)
		go func() {
			err := writeFrame(client, unboundRetiredFrame[:])
			_ = client.Close()
			written <- err
		}()
		if err := <-written; err != nil {
			t.Fatal(err)
		}
		// The actual service can be descheduled after its stop write. A
		// successful client write followed by Close must not lose the receipt.
		time.Sleep(time.Millisecond)
		var frame [8]byte
		_, err := io.ReadFull(server, frame[:])
		_ = server.Close()
		if err != nil || frame != unboundRetiredFrame {
			t.Fatal("final receipt was lost after a successful native write", err, frame)
		}
	}
}
