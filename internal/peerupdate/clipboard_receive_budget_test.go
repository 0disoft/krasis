package peerupdate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipboardReceiveStopsAtAdvertisedSize(t *testing.T) {
	for _, format := range []string{"", "png", "dib", "files"} {
		t.Run(clipboardContentType(format), func(t *testing.T) {
			m := testManager(t)
			m.generation = 72
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			_, _ = m.SyncClipboard(peer, nil, "")
			m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id,
				ClipboardFormat: format, ClipboardSize: 1})
			const sent = MaxClipboardBytes + 1
			body := &io.LimitedReader{R: strings.NewReader(strings.Repeat("x", sent)), N: sent}
			req := httptest.NewRequest(http.MethodPost, "/v1/clipboard", body)
			req.Header.Set("X-Krasis-ID", id)
			req.Header.Set("Content-Type", clipboardContentType(format))
			response := httptest.NewRecorder()
			m.handler(peer, func() bool { return true }).ServeHTTP(response, req)
			if response.Code != http.StatusConflict {
				t.Fatalf("oversized body: status=%d", response.Code)
			}
			if read := int64(sent) - body.N; read != 2 {
				t.Fatalf("read %d bytes for a 1-byte advertisement; want 2", read)
			}
			if value, err := m.SyncClipboard(peer, nil, ""); err != nil || value.ID != "" {
				t.Fatal("oversized body published", value.ID, err)
			}
		})
	}
}

func TestClipboardReceiveAdvertisedTextLength(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
		text string
		want bool
	}{
		{"empty", 0, "", true},
		{"exact", 3, "abc", true},
		{"short", 4, "abc", false},
		{"long", 2, "abc", false},
		{"negative", -1, "abc", false},
		{"above-format-limit", MaxClipboardBytes + 1, "abc", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			_, _ = m.SyncClipboard(peer, nil, "")
			m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: tc.size})
			body := strings.NewReader(tc.text)
			err := m.receiveClipboardPayload(peer, id, "", body, func() bool { return true }, true)
			if (err == nil) != tc.want {
				t.Fatalf("receive: %v, want accepted=%t", err, tc.want)
			}
			if (tc.size < 0 || tc.size > MaxClipboardBytes) && body.Len() != len(tc.text) {
				t.Fatal("invalid advertisement consumed body bytes")
			}
		})
	}
}
