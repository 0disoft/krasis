package localapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type appOpenOperations struct {
	*stubOperations
	calls int
}

func (o *appOpenOperations) OpenApp(context.Context, AppOpenRequest) (AppOpenResult, error) {
	o.calls++
	return AppOpenResult{Code: "queued"}, nil
}

func TestAppOpenRequiresAuthenticationAndRejectsExecutableArguments(t *testing.T) {
	token, peer := strings.Repeat("a", 64), strings.Repeat("b", 64)
	op := &appOpenOperations{stubOperations: &stubOperations{}}
	h := &handler{token: token, operations: op}
	for _, test := range []struct {
		body string
		auth bool
		want int
	}{
		{`{"action":"START","peer":"` + peer + `"}`, false, 401},
		{`{"action":"START","peer":"` + peer + `","executable":"other.exe"}`, true, 400},
		{`{"action":"START","peer":"` + peer + `","id":"forged"}`, true, 400},
		{`{"action":"STATUS","peer":"` + peer + `","id":"forged"}`, true, 400},
		{`{"action":"START","peer":"` + peer + `"}`, true, 200},
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/app-open", strings.NewReader(test.body))
		if test.auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, req)
		if response.Code != test.want {
			t.Fatalf("status=%d want=%d", response.Code, test.want)
		}
	}
	if op.calls != 1 {
		t.Fatal("unauthorized operation")
	}
}
