package localapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type upgradeStub struct {
	stubOperations
	stops      int
	generation int
	revision   string
}

func (s *upgradeStub) StopForUpgrade(_ context.Context, generation int, revision string) (OperationResult, error) {
	s.stops++
	s.generation, s.revision = generation, revision
	return OperationResult{SchemaVersion: ResultSchemaVersion, Status: "COMPLETED", Code: "upgrade_stop_requested"}, nil
}

func TestRuntimeAndUpgradeRequireAuthenticationAndExactInstance(t *testing.T) {
	info, err := newRuntimeInfo()
	if err != nil {
		t.Fatal(err)
	}
	ops := &upgradeStub{}
	h := &handler{token: strings.Repeat("a", 64), operations: ops, runtime: info}
	for _, test := range []struct {
		path, method, token, instance, body string
		want                                int
	}{
		{"/v1/runtime", "GET", "", "", "", 401},
		{"/v1/runtime", "GET", h.token, "", "", 200},
		{"/v1/upgrade-stop", "POST", "", info.Instance, "", 401},
		{"/v1/upgrade-stop", "POST", h.token, "wrong", "", 409},
		{"/v1/upgrade-stop", "POST", h.token, info.Instance, "{}", 405},
		{"/v1/upgrade-stop", "POST", h.token, info.Instance, "", 200},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer "+test.token)
		r.Header.Set("If-Match", test.instance)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("%s status %d", test.path, w.Code)
		}
	}
	if ops.stops != 1 {
		t.Fatalf("upgrade stops=%d", ops.stops)
	}
	r := httptest.NewRequest("POST", "/v1/upgrade-stop", nil)
	r.Header.Set("Authorization", "Bearer "+h.token)
	r.Header.Set("If-Match", info.Instance)
	r.Header.Set("X-Krasis-Upgrade-Generation", "83")
	r.Header.Set("X-Krasis-Upgrade-Build", strings.Repeat("b", 40))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || ops.stops != 2 || ops.generation != 83 || ops.revision != strings.Repeat("b", 40) {
		t.Fatal("update target was not bound to upgrade stop")
	}
	r.Header.Set("X-Krasis-Upgrade-Build", "invalid")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 || ops.stops != 2 {
		t.Fatal("invalid update target reached agent")
	}
}
