package localapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
