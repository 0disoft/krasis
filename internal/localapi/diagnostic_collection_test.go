package localapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type collectionOperations struct {
	*stubOperations
	calls int
}

func (o *collectionOperations) CollectDiagnostic(_ context.Context, request DiagnosticCollectionRequest) (DiagnosticCollectionResult, error) {
	o.calls++
	return DiagnosticCollectionResult{Code: "queued", ID: strings.Repeat("c", 32)}, nil
}

func TestDiagnosticCollectionRequiresLocalAuthenticationAndExactRequest(t *testing.T) {
	token := strings.Repeat("a", 64)
	peer := strings.Repeat("b", 64)
	op := &collectionOperations{stubOperations: &stubOperations{}}
	h := &handler{token: token, operations: op}
	for _, test := range []struct {
		body string
		auth bool
		want int
	}{
		{`{"action":"START","peer":"` + peer + `"}`, false, http.StatusUnauthorized},
		{`{"action":"START","peer":"` + peer + `","id":"forged"}`, true, http.StatusBadRequest},
		{`{"action":"STATUS","peer":"` + peer + `","id":"forged"}`, true, http.StatusBadRequest},
		{`{"action":"START","peer":"` + peer + `","unknown":true}`, true, http.StatusBadRequest},
		{`{"action":"START","peer":"` + peer + `"}`, true, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/diagnostic-collection", strings.NewReader(test.body))
		if test.auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != test.want {
			t.Fatalf("status %d for %q", w.Code, test.body)
		}
	}
	if op.calls != 1 {
		t.Fatalf("operations called %d times", op.calls)
	}
}
