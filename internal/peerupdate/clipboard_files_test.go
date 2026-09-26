package peerupdate

import (
	"archive/zip"
	"bytes"
	"encoding/json"
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

func TestClipboardFilesAboveLegacyLimitRequireGeneration72(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	// Store without compression to exercise the archive wire bound.
	data := make([]byte, 47<<20)
	archive := testClipboardZipBytes(t, "large.bin", data)
	if len(archive) <= legacyMaxClipboardFileBytes || !ValidClipboardFiles(archive) {
		t.Fatal("47 MiB file archive rejected")
	}
	if _, err := m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: archive}, ""); err != nil {
		t.Fatal(err)
	}
	m.observePeer(peer, PeerInfo{Generation: 71, Clipboard: true})
	if info := m.info(peer); info.ClipboardID != "" || info.ClipboardFormat != "" {
		t.Fatal("large file advertised to generation 71")
	}
	m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true})
	info := m.info(peer)
	if info.ClipboardID == "" || info.ClipboardSize != len(archive) {
		t.Fatal("large file not advertised to generation 72")
	}
	old := info
	old.Generation = 71
	data, _ = json.Marshal(old)
	if _, err := readPeerInfo(bytes.NewReader(data)); err == nil {
		t.Fatal("oversized archive accepted from generation 71")
	}
	data, _ = json.Marshal(info)
	if _, err := readPeerInfo(bytes.NewReader(data)); err != nil {
		t.Fatal("generation 72 archive rejected", err)
	}
	compressed := testClipboardZip(t, "large-compressible.bin", false, strings.Repeat("a", 41<<20))
	if len(compressed) >= legacyMaxClipboardFileBytes || !ValidClipboardFiles(compressed) {
		t.Fatal("compressed fixture is not within the wire limit")
	}
	if _, err := m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: compressed}, ""); err != nil {
		t.Fatal(err)
	}
	m.observePeer(peer, PeerInfo{Generation: 71, Clipboard: true})
	if info := m.info(peer); info.ClipboardID != "" {
		t.Fatal("oversized extracted files advertised to generation 71")
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

func testClipboardZipBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	h := &zip.FileHeader{Name: name, Method: zip.Store}
	h.SetMode(0600)
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
