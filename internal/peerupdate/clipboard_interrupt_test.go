package peerupdate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type delayedClipboardInterrupt struct {
	header                                       http.Header
	readStarted, readInterrupted, finishDeadline chan struct{}
	status                                       chan int
}

func (w *delayedClipboardInterrupt) Header() http.Header            { return w.header }
func (w *delayedClipboardInterrupt) WriteHeader(status int)         { w.status <- status }
func (w *delayedClipboardInterrupt) Write(data []byte) (int, error) { return len(data), nil }
func (w *delayedClipboardInterrupt) Read([]byte) (int, error) {
	close(w.readStarted)
	<-w.readInterrupted
	return 0, os.ErrDeadlineExceeded
}
func (w *delayedClipboardInterrupt) SetReadDeadline(time.Time) error {
	close(w.readInterrupted)
	<-w.finishDeadline
	return nil
}

func TestClipboardUploadJoinsDeadlineInterruptBeforeReturning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t)
		peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
		if err := m.SetClipboardPeer(peer); err != nil {
			t.Fatal(err)
		}
		if _, err := m.SyncClipboard(peer, nil, ""); err != nil {
			t.Fatal(err)
		}
		m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 3})
		writer := &delayedClipboardInterrupt{
			header: make(http.Header), readStarted: make(chan struct{}), readInterrupted: make(chan struct{}),
			finishDeadline: make(chan struct{}), status: make(chan int, 1),
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/clipboard", io.NopCloser(writer))
		request.Header.Set("X-Krasis-ID", id)
		request.Header.Set("Content-Type", clipboardContentType(""))
		done := make(chan struct{})
		go func() { defer close(done); m.handler(peer, func() bool { return true }).ServeHTTP(writer, request) }()
		<-writer.readStarted
		if err := m.SetClipboardPeer(""); err != nil {
			t.Fatal(err)
		}
		if status := <-writer.status; status != http.StatusConflict {
			t.Error("cancelled upload accepted", status)
		}
		synctest.Wait()
		select {
		case <-done:
			t.Error("request returned while its deadline callback could still affect the connection")
		default:
		}
		close(writer.finishDeadline)
		<-done
	})
}
