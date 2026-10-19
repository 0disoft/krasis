//go:build windows

package inputworker

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestNativePreparationWatcherJoinsWithoutClosingReceiptOrNextReader(t *testing.T) {
	server, client := nativePair(t)
	done := make(chan error, 1)
	go func() {
		if err := watchPreparationPeer(context.Background(), server, func(context.Context) error { return nil }); err != nil {
			done <- err
			return
		}
		// The watcher must be fully gone before the preparation receipt and
		// before a following read owns another buffer on this same stream.
		if err := server.SetDeadline(time.Now().Add(time.Second)); err != nil {
			done <- err
			return
		}
		if err := writeFrame(server, preparationReceipt[:]); err != nil {
			done <- err
			return
		}
		var next [1]byte
		_, err := io.ReadFull(server, next[:])
		if err == nil && next[0] != 7 {
			err = ErrProtocol
		}
		done <- err
	}()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	var receipt [8]byte
	if _, err := io.ReadFull(client, receipt[:]); err != nil || receipt != preparationReceipt {
		t.Fatal("native watcher prevented receipt", err)
	}
	if _, err := client.Write([]byte{7}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal("native watcher cancellation reached next I/O", err)
	}
}
