package peerupdate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClipboardTransportRequiresBothPermissionsAndAdvertisedID(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	post := func(id string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/clipboard", strings.NewReader("안녕"))
		req.Header.Set("X-Krasis-ID", id)
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		w := httptest.NewRecorder()
		m.handler(peer, func() bool { return true }).ServeHTTP(w, req)
		return w.Code
	}
	if post(id) != 403 {
		t.Fatal("remote permission missing")
	}
	m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 6})
	if post(strings.Repeat("c", 32)) != 400 {
		t.Fatal("unadvertised clipboard accepted")
	}
	if post(id) != 200 {
		t.Fatal("valid clipboard not queued")
	}
	value, _ := m.SyncClipboard(peer, nil, "")
	if value.Text != "안녕" {
		t.Fatal("text changed")
	}
	if err := m.SetClipboardPeer(""); err != nil {
		t.Fatal(err)
	}
	if post(id) != 403 {
		t.Fatal("revoked clipboard accepted")
	}
}

func TestClipboardOptInScopeExpiryAndEcho(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	id := strings.Repeat("c", 32)
	if _, err := m.SyncClipboard(peer, nil, ""); err == nil {
		t.Fatal("default allowed")
	}
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	if on, _, _ := m.ClipboardInfo(peer); on {
		t.Fatal("no GUI heartbeat")
	}
	if _, err := m.SyncClipboard(peer, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := m.clipboardReceive(other, id, strings.NewReader("private"), func() bool { return true }); err == nil {
		t.Fatal("wrong peer accepted")
	}
	if err := m.clipboardReceive(peer, id, strings.NewReader("hello"), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	value, err := m.SyncClipboard(peer, nil, "")
	if err != nil || value.Text != "hello" {
		t.Fatal(value, err)
	}
	if _, err := m.SyncClipboard(peer, nil, id); err != nil {
		t.Fatal(err)
	}
	if err := m.clipboardReceive(peer, id, strings.NewReader("hello"), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if value, _ := m.SyncClipboard(peer, nil, ""); value.ID != "" {
		t.Fatal("duplicate replay")
	}
	text := "local"
	if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
		t.Fatal(err)
	}
	if err := m.clipboardReceive(peer, strings.Repeat("d", 32), strings.NewReader("older"), func() bool { return true }); err != ErrBusy {
		t.Fatal("local copy overwritten")
	}
	m.clipboard.activeUntil = time.Now().Add(-time.Second)
	if on, _, _ := m.ClipboardInfo(peer); on {
		t.Fatal("expired GUI heartbeat")
	}
	if err := m.SetClipboardPeer(""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncClipboard(peer, nil, ""); err == nil {
		t.Fatal("revoked clipboard allowed")
	}
}
func TestClipboardRejectsInvalidRepresentations(t *testing.T) {
	for _, s := range []string{strings.Repeat("x", MaxClipboardBytes+1), "a\x00b", string([]byte{255})} {
		if validClipboardText(s) {
			t.Fatal("invalid text accepted")
		}
	}
	if !validClipboardText("안녕하세요\n@😊") {
		t.Fatal("Unicode rejected")
	}
}
