package localapi

import (
	"encoding/json"
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

func TestUpdateInspectionIsBoundedAndLegacyResponsesStayCompatible(t *testing.T) {
	peer := strings.Repeat("a", 64)
	if !(UpdateRequest{Schema: UpdateSchema, Action: "INSPECT", Peer: peer}).Valid() {
		t.Fatal("inspection rejected")
	}
	for _, r := range []UpdateRequest{{Schema: UpdateSchema, Action: "INSPECT"}, {Schema: UpdateSchema, Action: "INSPECT", Peer: peer, Directory: "private"}, {Schema: UpdateSchema, Action: "INSPECT", Peer: peer, ID: strings.Repeat("a", 32)}} {
		if r.Valid() {
			t.Fatal("cross-action fields accepted")
		}
	}
	raw, _ := json.Marshal(UpdateResult{Schema: UpdateSchema, Code: "ok"})
	if strings.Contains(string(raw), "peerInfo") {
		t.Fatal("legacy strict decoders would reject response")
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
