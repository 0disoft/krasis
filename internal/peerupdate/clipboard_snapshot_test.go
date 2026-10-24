package peerupdate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipboardSnapshotsKeepIndependentReadersAfterQueueChange(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	data := testClipboardZip(t, "file.txt", false, "original copy")
	want := bytes.Clone(data)
	if _, err := m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: data}, ""); err != nil {
		t.Fatal(err)
	}
	_, id, _ := m.ClipboardInfo(peer)
	first, second := m.clipboardOutgoing(peer, id), m.clipboardOutgoing(peer, id)
	if first == nil || second == nil || first.size != len(want) {
		t.Fatal("missing snapshot or wrong size")
	}
	prefix := make([]byte, 1)
	if _, err := io.ReadFull(first.body, prefix); err != nil || prefix[0] != want[0] {
		t.Fatal("cannot read first snapshot", err)
	}
	data[0] ^= 0xff // The enqueue caller still owns its original input slice.
	text := "replacement"
	if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
		t.Fatal(err)
	}
	for i, snapshot := range []*clipboardSnapshot{first, second} {
		expected := want
		if i == 0 {
			expected = want[1:]
		}
		got, err := io.ReadAll(snapshot.body)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("snapshot %d lost its original bytes or independent cursor: %v", i, err)
		}
	}
	if m.clipboardOutgoing(peer, id) != nil {
		t.Fatal("replaced queue entry still offered")
	}
}

func TestClipboardSnapshotFileDownloadAndUploadCarrySameBytes(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true})
	data := testClipboardZip(t, "file.txt", false, "file contents")
	if _, err := m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: data}, ""); err != nil {
		t.Fatal(err)
	}
	_, id, _ := m.ClipboardInfo(peer)
	request := httptest.NewRequest(http.MethodGet, "/v1/clipboard", nil)
	request.Header.Set("X-Krasis-ID", id)
	response := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data) {
		t.Fatal("file download changed snapshot bytes", response.Code)
	}
	if _, pending, _ := m.ClipboardInfo(peer); pending != id {
		t.Fatal("download acknowledged the copy before receipt")
	}
	uploads := 0
	client := &http.Client{Transport: clipboardTransportFunc(func(r *http.Request) (*http.Response, error) {
		uploads++
		got, err := io.ReadAll(r.Body)
		if err != nil || r.Method != http.MethodPost || r.Header.Get("X-Krasis-ID") != id ||
			r.Header.Get("Content-Type") != clipboardContentType("files") || !bytes.Equal(got, data) {
			t.Error("file upload changed snapshot bytes or metadata", err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	m.pollClipboard(context.Background(), client, "test.invalid", peer, func() bool { return true })
	if _, pending, _ := m.ClipboardInfo(peer); uploads != 1 || pending != "" {
		t.Fatal("successful upload did not acknowledge exactly one copy", uploads, pending)
	}
}
