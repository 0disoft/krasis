package localapi

import (
	"bytes"
	"encoding/json"
	"github.com/0disoft/krasis/internal/peerupdate"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipboardJSONExpansionRemainsBounded(t *testing.T) {
	text := strings.Repeat("<", peerupdate.MaxClipboardBytes)
	input := ClipboardRequest{Action: "SYNC", Peer: strings.Repeat("a", 64), Text: &text}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var result ClipboardRequest
	if err := decodeClipboardRequest(bytes.NewReader(data), &result); err != nil || !result.Valid() || *result.Text != text {
		t.Fatal("bounded text rejected", err)
	}
	if decodeClipboardRequest(strings.NewReader(strings.Repeat(" ", 6*peerupdate.MaxClipboardBytes+1025)), &result) == nil {
		t.Fatal("oversized request accepted")
	}
}

func TestSharingRoutesRequireAuthentication(t *testing.T) {
	h := &handler{token: strings.Repeat("a", 64)}
	for _, path := range []string{"/v1/clipboard", "/v1/diagnostic-transfer"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if w.Code != 401 {
			t.Fatal("unauthenticated", path, w.Code)
		}
	}
}
func TestClipboardRequestActionIsolation(t *testing.T) {
	peer := strings.Repeat("a", 64)
	text := "hello"
	for _, r := range []ClipboardRequest{{Action: "ENABLE", Peer: peer, Text: &text}, {Action: "SYNC", Peer: "bad"}, {Action: "SYNC", Peer: peer, Ack: "bad"}, {Action: "EXEC", Peer: peer}} {
		if r.Valid() {
			t.Fatal("invalid request")
		}
	}
}

func TestClipboardImageRequestCannotMasqueradeAsTextOrPermission(t *testing.T) {
	peer := strings.Repeat("a", 64)
	data := []byte{1, 2, 3}
	text := "private"
	for _, request := range []ClipboardRequest{
		{Action: "ENABLE", Peer: peer, Format: "png", Data: data},
		{Action: "SYNC", Peer: peer, Format: "png", Text: &text, Data: data},
		{Action: "SYNC", Peer: peer, Data: data},
	} {
		if request.Valid() {
			t.Fatal("invalid image request accepted")
		}
	}
}
