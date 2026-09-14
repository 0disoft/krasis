package localapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type upgradeStub struct {
	stubOperations
	stops int
}

func (s *upgradeStub) StopForUpgrade(context.Context) (OperationResult, error) {
	s.stops++
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
}
