package localapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type clipboardWaitOperations struct {
	stubOperations
	revision uint64
}

func (o *clipboardWaitOperations) WaitClipboard(ctx context.Context, revision uint64) ClipboardWaitResult {
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded wait")
	}
	o.revision = revision
	return ClipboardWaitResult{Code: "ok", Revision: revision + 1}
}

func TestClipboardWaitAuthenticatedStrictAndOptional(t *testing.T) {
	op := &clipboardWaitOperations{}
	h := &handler{token: strings.Repeat("a", 64), operations: op}
	call := func(body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/clipboard/wait", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.Header.Set("Authorization", "Bearer "+h.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call(`{"revision":1}`, false); w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"revision":-1}`, `{"revision":"x"}`, `{"extra":1}`, `{} {}`} {
		if w := call(body, true); w.Code != http.StatusBadRequest {
			t.Fatal(body, w.Code)
		}
	}
	if w := call(`{"revision":7}`, true); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"revision":8`) || op.revision != 7 {
		t.Fatal(w.Body.String())
	}
	h.operations = &stubOperations{}
	if w := call(`{"revision":0}`, true); !strings.Contains(w.Body.String(), `"code":"unsupported"`) {
		t.Fatal(w.Body.String())
	}
}
