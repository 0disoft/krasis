package peerupdate

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockedClipboardWriter struct {
	header               http.Header
	started, interrupted chan struct{}
	once                 sync.Once
}

func (w *blockedClipboardWriter) Header() http.Header { return w.header }
func (w *blockedClipboardWriter) WriteHeader(int)     {}
func (w *blockedClipboardWriter) Write([]byte) (int, error) {
	close(w.started)
	<-w.interrupted
	return 0, os.ErrDeadlineExceeded
}
func (w *blockedClipboardWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		w.once.Do(func() { close(w.interrupted) })
	}
	return nil
}

func TestClipboardDownloadReleasesGateWhenCopyOrPermissionChanges(t *testing.T) {
	for _, change := range []string{"new-copy", "local-disabled", "remote-disabled"} {
		t.Run(change, func(t *testing.T) {
			m := testManager(t)
			peer := strings.Repeat("a", 64)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			m.observePeer(peer, PeerInfo{Clipboard: true})
			text := "old copy"
			if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
				t.Fatal(err)
			}
			_, id, _ := m.ClipboardInfo(peer)
			handler := m.handler(peer, func() bool { return true })
			writer := &blockedClipboardWriter{header: make(http.Header), started: make(chan struct{}), interrupted: make(chan struct{})}
			request := httptest.NewRequest(http.MethodGet, "/v1/clipboard", nil)
			request.Header.Set("X-Krasis-ID", id)
			done := make(chan struct{})
			go func() { defer close(done); handler.ServeHTTP(writer, request) }()
			defer func() { _ = writer.SetWriteDeadline(time.Now()); <-done }()
			<-writer.started
			switch change {
			case "new-copy":
				text = "new copy"
				if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
					t.Fatal(err)
				}
			case "local-disabled":
				if err := m.SetClipboardPeer(""); err != nil {
					t.Fatal(err)
				}
			case "remote-disabled":
				m.observePeer(peer, PeerInfo{Clipboard: false})
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("obsolete download retained the clipboard transfer gate")
			}
			if change == "new-copy" {
				_, next, _ := m.ClipboardInfo(peer)
				request = httptest.NewRequest(http.MethodGet, "/v1/clipboard", nil)
				request.Header.Set("X-Krasis-ID", next)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK || response.Body.String() != text {
					t.Fatal("new copy remained blocked after obsolete transfer", response.Code)
				}
				_, stillPending, _ := m.ClipboardInfo(peer)
				if stillPending != next {
					t.Fatal("download alone acknowledged delivery")
				}
			}
		})
	}
}
