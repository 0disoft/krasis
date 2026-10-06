package localapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type releaseClaimStub struct {
	stubOperations
	calls int
}

func (s *releaseClaimStub) ReleaseClaim(context.Context, string, int, string, bool) (OperationResult, error) {
	s.calls++
	return OperationResult{SchemaVersion: ResultSchemaVersion, Status: "COMPLETED", Code: "release_claimed"}, nil
}
func TestReleaseClaimRequiresAuthenticationInstanceAndNewTarget(t *testing.T) {
	s := &releaseClaimStub{}
	h := &handler{operations: s, token: strings.Repeat("a", 64), runtime: RuntimeInfo{Instance: strings.Repeat("b", 32), Generation: 186}}
	for _, tc := range []struct {
		token, instance, generation string
		status                      int
	}{
		{"", h.runtime.Instance, "187", 401}, {h.token, "wrong", "187", 409}, {h.token, h.runtime.Instance, "186", 400}, {h.token, h.runtime.Instance, "187", 200},
	} {
		r := httptest.NewRequest("POST", "/v1/release-claim", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("If-Match", tc.instance)
		r.Header.Set("X-Krasis-Release-ID", strings.Repeat("c", 32))
		r.Header.Set("X-Krasis-Release-Action", "CLAIM")
		r.Header.Set("X-Krasis-Upgrade-Generation", tc.generation)
		r.Header.Set("X-Krasis-Upgrade-Build", strings.Repeat("d", 40))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(w.Code, tc.status)
		}
	}
	if s.calls != 1 {
		t.Fatal(s.calls)
	}
}
