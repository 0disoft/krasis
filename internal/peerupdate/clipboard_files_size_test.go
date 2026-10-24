package peerupdate

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipboardFilesRejectExpandedSizeOverflow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sizes []uint64
		valid bool
	}{
		{"single_at_limit", []uint64{MaxClipboardFileBytes}, true},
		{"sum_at_limit", []uint64{MaxClipboardFileBytes - 1, 1}, true},
		{"single_over_limit", []uint64{MaxClipboardFileBytes + 1}, false},
		{"sum_over_limit", []uint64{MaxClipboardFileBytes, 1}, false},
		{"wrap_to_zero", []uint64{1, ^uint64(0)}, false},
		{"wrap_to_one", []uint64{2, ^uint64(0)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := clipboardSizeMetadataZip(t, tc.sizes...)
			if got := ValidClipboardFiles(data); got != tc.valid {
				t.Fatalf("%d-byte ZIP with expanded sizes %v: accepted=%t, want %t", len(data), tc.sizes, got, tc.valid)
			}
		})
	}
}

func TestClipboardFilesHTTPRejectsExpandedSizeOverflow(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	data := clipboardSizeMetadataZip(t, 1, ^uint64(0))
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncClipboardPayload(peer, nil, ""); err != nil {
		t.Fatal(err)
	}
	m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id,
		ClipboardFormat: "files", ClipboardSize: len(data)})
	req := httptest.NewRequest(http.MethodPost, "/v1/clipboard", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/x-krasis-files+zip")
	req.Header.Set("X-Krasis-ID", id)
	response := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(response, req)
	if response.Code != http.StatusConflict {
		t.Fatalf("overflowed clipboard receive: HTTP %d, want 409", response.Code)
	}
	value, err := m.SyncClipboardPayload(peer, nil, "")
	if err != nil || value.ID != "" {
		t.Fatal("rejected archive reached the clipboard queue", value.ID, err)
	}
}

// This fixture exercises metadata admission without allocating expanded data.
func clipboardSizeMetadataZip(t *testing.T, sizes ...uint64) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for i, size := range sizes {
		header := &zip.FileHeader{Name: fmt.Sprintf("file%d.bin", i), Method: zip.Store, UncompressedSize64: size}
		header.SetMode(0600)
		if _, err := w.CreateRaw(header); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := out.Bytes()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) != len(sizes) {
		t.Fatalf("invalid size metadata fixture: %v", err)
	}
	for i, file := range reader.File {
		if file.UncompressedSize64 != sizes[i] {
			t.Fatalf("fixture entry %d has size %d, want %d", i, file.UncompressedSize64, sizes[i])
		}
	}
	return data
}
