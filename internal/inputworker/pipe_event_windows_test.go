//go:build windows

package inputworker

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativePipeCompletionEventsSurviveCancellationAndStaySeparate(t *testing.T) {
	server, client := nativePair(t)
	if err := server.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if _, err := server.Read(data[:]); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("pending read did not cancel", err)
	}
	readEvent := server.readEvent
	if readEvent == 0 {
		t.Fatal("read has no completion event")
	}
	// A cancelled or completed operation may leave a manual-reset event set.
	// The next pending read must not interpret that old signal as completion.
	if err := windows.SetEvent(readEvent); err != nil {
		t.Fatal(err)
	}
	if err := server.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	read := make(chan error, 1)
	go func() { _, err := io.ReadFull(server, data[:]); read <- err }()
	// Duplex writes must neither reset nor signal the pending read's event.
	if _, err := server.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	var response [1]byte
	if _, err := io.ReadFull(client, response[:]); err != nil || response[0] != 3 {
		t.Fatal("duplex write failed", err)
	}
	if _, err := client.Write([]byte{7}); err != nil {
		t.Fatal(err)
	}
	if err := <-read; err != nil || data[0] != 7 {
		t.Fatal("stale completion signal broke the next read", err)
	}
	writeEvent := server.writeEvent
	if server.readEvent != readEvent || writeEvent == 0 || writeEvent == readEvent {
		t.Fatal("duplex operations replaced or shared their completion events")
	}
	if _, err := server.Write([]byte{9}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(client, response[:]); err != nil || response[0] != 9 || server.writeEvent != writeEvent {
		t.Fatal("second write did not reuse its own event", err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	for _, event := range []windows.Handle{readEvent, writeEvent} {
		if _, err := windows.WaitForSingleObject(event, 0); err != windows.ERROR_INVALID_HANDLE {
			t.Fatal("completion event survived pipe close", err)
		}
	}
}
