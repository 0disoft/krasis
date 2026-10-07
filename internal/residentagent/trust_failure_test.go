package residentagent

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/productlog"
)

func TestTrustMonitorFailureRetainsExportableCause(t *testing.T) {
	for code, want := range map[string]string{
		"session_trust_freshness_expired": "fault_trust_freshness_expired",
		"session_trust_revoked":           "fault_trust_revoked",
		"session_trust_peer_missing":      "fault_trust_missing",
		"session_trust_store_missing":     "fault_trust_missing",
		"session_trust_check_failed":      "fault_trust_check_failed",
	} {
		failure := agent.DescribeSessionFailure(errors.Join(errors.New("private"), sessionTrustFailure(code)))
		if got := productlog.FaultCode(failure.Component, failure.Phase, failure.Code); got != want {
			t.Fatalf("%s: %s", code, got)
		}
	}
	if sessionTrustFailure("session_trust_read_deferred") != nil {
		t.Fatal("soft wait became a terminal verdict")
	}
}
