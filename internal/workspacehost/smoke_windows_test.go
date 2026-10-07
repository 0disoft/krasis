//go:build windows && desktop

package workspacehost

import (
	"github.com/0disoft/krasis/internal/productpanel"
	"testing"
)

func TestSmokeSeparatesWorkingBridgeFromFaultedAgent(t *testing.T) {
	for _, tc := range []struct {
		state, code        string
		roundtrip, healthy bool
	}{
		{"CONNECTED", "ok", true, true},
		{"FAULTED", "recovery_required", true, false},
		{"FAULTED", "ok", true, false},
		{"UNKNOWN", "unavailable", false, false},
		{"private", "private", false, false},
	} {
		s := &WorkspaceSmoke{result: make(chan workspaceSmokeResult, 1)}
		s.Report(productpanel.BridgeSchema, tc.state, tc.code)
		got := <-s.result
		if got.Roundtrip != tc.roundtrip || got.Healthy != tc.healthy {
			t.Fatal(tc, got)
		}
		if !got.Roundtrip && (got.State != "UNKNOWN" || got.Code != "unavailable") {
			t.Fatal("raw smoke values escaped", got)
		}
	}
}
