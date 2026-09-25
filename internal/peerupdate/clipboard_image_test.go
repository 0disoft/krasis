package peerupdate

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipboardImageValidation(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if !ValidClipboardImage("png", encoded.Bytes()) || ValidClipboardImage("png", encoded.Bytes()[:10]) ||
		ValidClipboardImage("jpeg", encoded.Bytes()) {
		t.Fatal("PNG format validation")
	}
	dib := make([]byte, 40+16)
	binary.LittleEndian.PutUint32(dib[:4], 40)
	binary.LittleEndian.PutUint32(dib[4:8], 2)
	binary.LittleEndian.PutUint32(dib[8:12], 2)
	binary.LittleEndian.PutUint16(dib[12:14], 1)
	binary.LittleEndian.PutUint16(dib[14:16], 32)
	if !ValidClipboardImage("dib", dib) || ValidClipboardImage("dib", dib[:len(dib)-1]) {
		t.Fatal("DIB bounds validation")
	}
	binary.LittleEndian.PutUint32(dib[4:8], 0xffffffff)
	if ValidClipboardImage("dib", dib) {
		t.Fatal("negative image width accepted")
	}
}

func TestClipboardImageRequiresCurrentPeerAndKeepsBytesOutOfStatus(t *testing.T) {
	m := testManager(t)
	m.generation = 69
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("b", 32)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	m.observePeer(peer, PeerInfo{Generation: 69, Clipboard: true, ClipboardID: id,
		ClipboardSize: encoded.Len(), ClipboardFormat: "png"})
	req := httptest.NewRequest(http.MethodPost, "/v1/clipboard", bytes.NewReader(encoded.Bytes()))
	req.Header.Set("Content-Type", "image/png")
	req.Header.Set("X-Krasis-ID", id)
	w := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("image receive: %d", w.Code)
	}
	value, err := m.SyncClipboardPayload(peer, nil, "")
	if err != nil || value.Format != "png" || !bytes.Equal(value.Data, encoded.Bytes()) {
		t.Fatal("image changed in inbox", err)
	}
	value.Data[0] ^= 0xff
	again, _ := m.SyncClipboardPayload(peer, nil, "")
	if bytes.Equal(value.Data, again.Data) {
		t.Fatal("inbox data shared with caller")
	}
	_, _ = m.SyncClipboardPayload(peer, nil, id)
	_, err = m.SyncClipboardPayload(peer, &ClipboardValue{Format: "png", Data: encoded.Bytes()}, "")
	if err != nil {
		t.Fatal(err)
	}
	info := m.info(peer)
	if info.ClipboardFormat != "png" || info.ClipboardSize != encoded.Len() || info.ClipboardID == "" {
		t.Fatal("image metadata missing")
	}
	m.observePeer(peer, PeerInfo{Generation: 68, Clipboard: true})
	info = m.info(peer)
	if info.ClipboardID != "" || info.ClipboardSize != 0 || info.ClipboardFormat != "" {
		t.Fatal("image advertised to text-only peer")
	}
}

func TestClipboardImageNeedsFreshConsentAfterTextOnlyVersion(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	m.generation = 68
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	m.generation = 69
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	value := &ClipboardValue{Format: "png", Data: encoded.Bytes()}
	if _, err := m.SyncClipboardPayload(peer, value, ""); err != ErrDenied {
		t.Fatalf("old text consent allowed image: %v", err)
	}
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncClipboardPayload(peer, value, ""); err != nil {
		t.Fatal(err)
	}
}
