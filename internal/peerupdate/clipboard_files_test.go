package peerupdate

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClipboardFilesNeedNewConsentAndNewPeer(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	files := testClipboardZip(t, "note.txt", false, "actual bytes")
	m.generation = 69
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	m.generation = 70
	value := &ClipboardValue{Format: "files", Data: files}
	if _, err := m.SyncClipboardPayload(peer, value, ""); err != ErrDenied {
		t.Fatalf("older media consent allowed files: %v", err)
	}
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncClipboardPayload(peer, value, ""); err != nil {
		t.Fatal(err)
	}
	m.observePeer(peer, PeerInfo{Generation: 69, Clipboard: true})
	if info := m.info(peer); info.ClipboardID != "" || info.ClipboardFormat != "" {
		t.Fatal("advertised files to old peer")
	}
	m.observePeer(peer, PeerInfo{Generation: 70, Clipboard: true})
	if info := m.info(peer); info.ClipboardID == "" || info.ClipboardFormat != "files" {
		t.Fatal("new peer did not see files")
	}
}

func TestClipboardFilesTransportCarriesBytes(t *testing.T) {
	m := testManager(t)
	m.generation = 70
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("b", 32)
	data := testClipboardZip(t, "note.txt", false, "actual bytes")
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboardPayload(peer, nil, "")
	m.observePeer(peer, PeerInfo{Generation: 70, Clipboard: true, ClipboardID: id,
		ClipboardFormat: "files", ClipboardSize: len(data)})
	req := httptest.NewRequest(http.MethodPost, "/v1/clipboard", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/x-krasis-files+zip")
	req.Header.Set("X-Krasis-ID", id)
	w := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("file receive: %d", w.Code)
	}
	value, err := m.SyncClipboardPayload(peer, nil, "")
	if err != nil || value.Format != "files" || !bytes.Equal(value.Data, data) {
		t.Fatal("file bytes changed", err)
	}
}

func TestClipboardFilesRejectUnsafeArchives(t *testing.T) {
	for _, name := range []string{"../escape", "C:/private", "folder\\file", "CON.txt", "a/../b", "trailing. ", "foo:stream"} {
		if ValidClipboardFiles(testClipboardZip(t, name, false)) {
			t.Fatalf("accepted unsafe name %q", name)
		}
	}
	if ValidClipboardFiles(testClipboardZip(t, "link", true)) {
		t.Fatal("accepted symlink")
	}
	if ValidClipboardFiles(testClipboardZip(t, "big", false, strings.Repeat("a", MaxClipboardFileBytes+1))) {
		t.Fatal("accepted decompression bomb")
	}
}

func testClipboardZip(t *testing.T, name string, symlink bool, content ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	if symlink {
		h.SetMode(os.ModeSymlink | 0777)
	}
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 0 {
		if _, err := f.Write([]byte(content[0])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
