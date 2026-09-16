package localapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateRouteRequiresLocalAuthenticationBeforeDispatch(t *testing.T) {
	h := &handler{token: strings.Repeat("a", 64)}
	r := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(`{"schema":"krasis.peer-update/v1","action":"ALLOW"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestUpdateRequestRejectsUnknownActionsAndCrossActionFields(t *testing.T) {
	peer := strings.Repeat("a", 64)
	for _, r := range []UpdateRequest{
		{Schema: UpdateSchema, Action: "STATUS", Peer: peer},
		{Schema: UpdateSchema, Action: "ALLOW", Peer: peer, Directory: "C:/other"},
		{Schema: UpdateSchema, Action: "SEND", Peer: peer, Directory: "bad\x00path"},
		{Schema: UpdateSchema, Action: "APPLY", Peer: peer, ID: "../escape"},
		{Schema: UpdateSchema, Action: "EXEC", Peer: peer},
	} {
		if r.Valid() {
			t.Fatalf("accepted: %+v", r)
		}
	}
	if !(UpdateRequest{Schema: UpdateSchema, Action: "ALLOW", Peer: peer}).Valid() {
		t.Fatal("valid permission rejected")
	}
}
