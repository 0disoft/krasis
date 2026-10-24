package peerupdate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestClipboardTransferContextOwnsDeadline(t *testing.T) {
	for _, format := range []string{"", "png", "dib", "files"} {
		t.Run(clipboardContentType(format), func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardFormat: format, ClipboardSize: 3})
			start := time.Now()
			ctx, stop, _ := m.clipboardTransferContext(context.Background(), peer, id, format, 3, false)
			defer stop()
			deadline, ok := ctx.Deadline()
			budget := clipboardTransferTimeout(format)
			if !ok || deadline.Before(start.Add(budget)) || deadline.After(time.Now().Add(budget)) {
				t.Fatal("transfer has no format-specific deadline", deadline, ok)
			}
			parent, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			child, stopChild, _ := m.clipboardTransferContext(parent, peer, id, format, 3, false)
			defer stopChild()
			parentDeadline, _ := parent.Deadline()
			if childDeadline, ok := child.Deadline(); !ok || !childDeadline.Equal(parentDeadline) {
				t.Fatal("transfer extended its parent's shorter deadline")
			}
		})
	}
}

func TestClipboardUploadTimeoutReleasesReceiveSlot(t *testing.T) {
	for _, format := range []string{"", "png", "dib", "files"} {
		t.Run(clipboardContentType(format), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := testManager(t)
				m.generation = 72
				peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
				if err := m.SetClipboardPeer(peer); err != nil {
					t.Fatal(err)
				}
				_, _ = m.SyncClipboard(peer, nil, "")
				m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id, ClipboardFormat: format, ClipboardSize: 3})
				writer := &delayedClipboardInterrupt{
					header: make(http.Header), readStarted: make(chan struct{}), readInterrupted: make(chan struct{}),
					finishDeadline: make(chan struct{}), status: make(chan int, 1),
				}
				close(writer.finishDeadline)
				request := httptest.NewRequest(http.MethodPost, "/v1/clipboard", io.NopCloser(writer))
				request.Header.Set("X-Krasis-ID", id)
				request.Header.Set("Content-Type", clipboardContentType(format))
				done := make(chan struct{})
				go func() { defer close(done); m.handler(peer, func() bool { return true }).ServeHTTP(writer, request) }()
				<-writer.readStarted
				budget := clipboardTransferTimeout(format)
				time.Sleep(budget - time.Nanosecond)
				synctest.Wait()
				select {
				case <-done:
					t.Error("upload interrupted before its format-specific deadline")
				default:
				}
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Error("stalled upload retained the receive slot beyond its deadline")
					_ = writer.SetReadDeadline(time.Now())
					<-done
				}
				if status := <-writer.status; status != http.StatusConflict {
					t.Error("timed-out upload was accepted", status)
				}
				// A new valid upload must succeed after the timed-out reader leaves.
				nextID := strings.Repeat("c", 32)
				_, _ = m.SyncClipboard(peer, nil, "")
				m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: nextID, ClipboardSize: 3})
				next := httptest.NewRequest(http.MethodPost, "/v1/clipboard", strings.NewReader("abc"))
				next.Header.Set("X-Krasis-ID", nextID)
				next.Header.Set("Content-Type", clipboardContentType(""))
				response := httptest.NewRecorder()
				m.handler(peer, func() bool { return true }).ServeHTTP(response, next)
				if response.Code != http.StatusOK {
					t.Fatal("timed-out upload leaked its receive slot", response.Code)
				}
			})
		})
	}
}
