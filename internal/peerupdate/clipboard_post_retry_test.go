package peerupdate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipboardPOSTRetryDoesNotReadReceivedFilesAgain(t *testing.T) {
	sender, receiver := testManager(t), testManager(t)
	sender.generation, receiver.generation = 72, 72
	senderID, receiverID := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, pair := range []struct {
		m    *Manager
		peer string
	}{{sender, receiverID}, {receiver, senderID}} {
		if err := pair.m.SetClipboardPeer(pair.peer); err != nil {
			t.Fatal(err)
		}
		_, _ = pair.m.SyncClipboardPayload(pair.peer, nil, "")
	}
	data := testClipboardZipBytes(t, "file.bin", bytes.Repeat([]byte("x"), 1<<20))
	if _, err := sender.SyncClipboardPayload(receiverID, &ClipboardValue{Format: "files", Data: data}, ""); err != nil {
		t.Fatal(err)
	}
	_, id, size, format, _ := sender.ClipboardPayloadInfo(receiverID)
	sender.observePeer(receiverID, PeerInfo{Generation: 72, Clipboard: true})
	receiver.observePeer(senderID, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id,
		ClipboardFormat: format, ClipboardSize: size})
	handler := receiver.handler(senderID, func() bool { return true })
	posts, read := 0, 0
	client := &http.Client{Transport: clipboardTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/clipboard" || r.Header.Get("X-Krasis-ID") != id {
			t.Fatal("unexpected clipboard retry request", r.Method, r.URL.Path)
		}
		posts++
		r.Body = io.NopCloser(countedClipboardBody{Reader: r.Body, read: &read})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != http.StatusOK {
			t.Fatal("clipboard POST rejected", response.Code)
		}
		if posts < 4 {
			return nil, errors.New("injected lost clipboard success response")
		}
		return response.Result(), nil
	})}
	for attempt := 0; attempt < 4; attempt++ {
		sender.pollClipboard(context.Background(), client, "test.invalid", receiverID, func() bool { return true })
	}
	if posts != 4 || read != len(data) {
		t.Fatalf("POST=%d payload=%d; want POST=4 payload=%d", posts, read, len(data))
	}
	if state := sender.ClipboardTransferStatus().State; state != "SENT" {
		t.Fatal("successful retry did not finish sender", state)
	}
	value, err := receiver.SyncClipboardPayload(senderID, nil, "")
	if err != nil || value.ID != id || !bytes.Equal(value.Data, data) {
		t.Fatal("retry changed received file copy", value.ID, err)
	}
	// Applying the copy is terminal too; a later POST must not requeue it.
	if _, err = receiver.SyncClipboardPayload(senderID, nil, id); err != nil {
		t.Fatal(err)
	}
	retry := httptest.NewRequest(http.MethodPost, "/v1/clipboard", bytes.NewReader(data))
	retry.Header.Set("X-Krasis-ID", id)
	retry.Header.Set("Content-Type", clipboardContentType(format))
	readApplied := 0
	retry.Body = io.NopCloser(countedClipboardBody{Reader: retry.Body, read: &readApplied})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, retry)
	if response.Code != http.StatusOK || readApplied != 0 || receiver.ClipboardTransferStatus().State != "APPLIED" {
		t.Fatal("applied copy was read again or regressed", response.Code, readApplied)
	}
	// Identical bytes under a new copy ID must still be read and queued.
	if _, err = sender.SyncClipboardPayload(receiverID, &ClipboardValue{Format: "files", Data: data}, ""); err != nil {
		t.Fatal(err)
	}
	_, id, _, _, _ = sender.ClipboardPayloadInfo(receiverID)
	receiver.observePeer(senderID, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id,
		ClipboardFormat: format, ClipboardSize: size})
	sender.pollClipboard(context.Background(), client, "test.invalid", receiverID, func() bool { return true })
	value, err = receiver.SyncClipboardPayload(senderID, nil, "")
	if posts != 5 || read != 2*len(data) || err != nil || value.ID != id || !bytes.Equal(value.Data, data) {
		t.Fatal("new copy skipped its body", posts, read, value.ID, err)
	}
}

func TestClipboardTerminalPOSTRechecksAuthorityBeforeReceipt(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 3})
	if err := m.receiveClipboardPayload(peer, id, "", strings.NewReader("abc"), func() bool { return true }, true); err != nil {
		t.Fatal(err)
	}
	checks, read := 0, 0
	body := countedClipboardBody{Reader: strings.NewReader("abc"), read: &read}
	err := m.receiveClipboardPayload(peer, id, "", body, func() bool { checks++; return checks == 1 }, true)
	if !errors.Is(err, ErrDenied) || read != 0 {
		t.Fatal("terminal receipt bypassed live authority or consumed body", read, err)
	}
}
