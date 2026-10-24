package peerupdate

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
)

type clipboardAdmissionReader struct {
	read func([]byte) (int, error)
}

func (r clipboardAdmissionReader) Read(p []byte) (int, error) { return r.read(p) }

func TestClipboardReceiveAdmissionSpansHandlersAndPolls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t)
		peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
		if err := m.SetClipboardPeer(peer); err != nil {
			t.Fatal(err)
		}
		_, _ = m.SyncClipboard(peer, nil, "")
		m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 3})
		started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		body, reads := strings.NewReader("abc"), 0
		first := clipboardAdmissionReader{read: func(p []byte) (int, error) {
			reads++
			if reads == 1 {
				return body.Read(p[:1])
			}
			if reads == 2 {
				close(started)
				<-release
			}
			return body.Read(p)
		}}
		go func() {
			done <- m.receiveClipboardPayload(peer, id, "", first, func() bool { return true }, true)
		}()
		defer func() {
			close(release)
			if err := <-done; err != nil {
				t.Error("admitted receive failed", err)
			}
			nextID := strings.Repeat("c", 32)
			m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: nextID, ClipboardSize: 3})
			if err := m.receiveClipboardPayload(peer, nextID, "", strings.NewReader("xyz"), func() bool { return true }, true); err != nil {
				t.Error("successful receive leaked admission", err)
			}
		}()
		synctest.Wait()
		select {
		case <-started:
		default:
			t.Fatal("first receive did not reach body read")
		}
		for attempt := 0; attempt < 2; attempt++ {
			read := 0
			req := httptest.NewRequest(http.MethodPost, "/v1/clipboard", countedClipboardBody{Reader: strings.NewReader("abc"), read: &read})
			req.Header.Set("X-Krasis-ID", id)
			req.Header.Set("Content-Type", clipboardContentType(""))
			response := httptest.NewRecorder()
			// A fresh handler has its own HTTP gate, but shares the Manager.
			m.handler(peer, func() bool { return true }).ServeHTTP(response, req)
			if response.Code != http.StatusConflict || read != 0 {
				t.Fatalf("duplicate body admitted: status=%d bytes=%d", response.Code, read)
			}
			if state := m.ClipboardTransferStatus().State; state != "RECEIVING" {
				t.Fatal("rejected duplicate changed active receive state", state)
			}
		}
		if err := m.receiveClipboardPayload(peer, id, "", strings.NewReader("abc"), func() bool { return false }, true); !errors.Is(err, ErrDenied) {
			t.Fatal("unauthorized duplicate accepted", err)
		}
		if state := m.ClipboardTransferStatus().State; state != "RECEIVING" {
			t.Fatal("unauthorized duplicate failed the active receiver", state)
		}
	})
}

func TestClipboardReceiveAdmissionReleasedAfterFailure(t *testing.T) {
	for _, mode := range []string{"read", "invalid payload", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			_, _ = m.SyncClipboard(peer, nil, "")
			var reader io.Reader = strings.NewReader("abc")
			checks := 0
			authorized := func() bool {
				checks++
				return mode != "revoked" || checks == 1
			}
			if mode == "read" {
				reader = clipboardAdmissionReader{read: func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }}
			} else if mode == "invalid payload" {
				reader = strings.NewReader("\x00")
			}
			if err := m.clipboardReceivePayload(peer, id, "", reader, authorized); !errors.Is(err, ErrDenied) {
				t.Fatal("failed receive accepted", err)
			}
			if err := m.clipboardReceivePayload(peer, id, "", strings.NewReader("abc"), func() bool { return true }); err != nil {
				t.Fatal("failed receive leaked admission", err)
			}
			value, err := m.SyncClipboard(peer, nil, "")
			if err != nil || value.ID != id || value.Text != "abc" {
				t.Fatal("retry did not publish the clipboard", value, err)
			}
		})
	}
}
