package localapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectionClientRejectsIncompleteSuccess(t *testing.T) {
	client := &Client{
		credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1234"}, Token: strings.Repeat("a", 64)},
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"schemaVersion":"krasis.agent.connection/v1","status":"COMPLETED","code":"connection_loaded"}`))}, nil
		})},
	}
	if _, err := client.Connection(context.Background(), ConnectionRequest{Action: "SHOW", PeerAlias: "peer"}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("incomplete success: %v", err)
	}
}

func TestConnectionRouteRejectsUnauthenticatedAndInvalidRequests(t *testing.T) {
	token := strings.Repeat("a", 64)
	// Invalid requests must return before invoking any operation implementation.
	handler := &handler{token: token}
	for _, test := range []struct {
		body          string
		authenticated bool
		status        int
	}{
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET_REARM","peerAlias":"peer","fingerprint":"` + strings.Repeat("a", 64) + `","rearmDistance":64}`, false, http.StatusUnauthorized},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET_REARM","peerAlias":"peer","fingerprint":"` + strings.Repeat("a", 64) + `","rearmDistance":1}`, true, http.StatusBadRequest},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET_REARM","peerAlias":"peer","fingerprint":"` + strings.Repeat("a", 64) + `","rearmDistance":64,"edge":"LEFT"}`, true, http.StatusBadRequest},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET_EDGE","peerAlias":"peer","edge":"LEFT"}`, false, http.StatusUnauthorized},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET_EDGE","peerAlias":"peer","edge":"LEFT"}`, true, http.StatusBadRequest},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET_EDGE","peerAlias":"peer","fingerprint":"` + strings.Repeat("a", 64) + `","edge":"LEFT","address":"127.0.0.1:4567"}`, true, http.StatusBadRequest},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SHOW","peerAlias":"peer"}`, false, http.StatusUnauthorized},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SET","peerAlias":"peer","address":"8.8.8.8:1234","edge":"RIGHT"}`, true, http.StatusBadRequest},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SHOW","peerAlias":"peer","allowInboundControl":true}`, true, http.StatusBadRequest},
		{`{"schemaVersion":"krasis.agent.connection/v1","action":"SHOW","peerAlias":"peer"} {}`, true, http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/connection", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		if test.authenticated {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
		}
	}
}
