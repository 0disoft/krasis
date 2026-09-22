package productlog

import (
	"strings"
	"testing"
)

func TestProductMonitorReasonsSurviveExport(t *testing.T) {
	for _, code := range []string{"session_certificate_deadline", "session_guard_exited", "session_guard_close_failed", "session_trust_check_failed", "session_trust_busy_exhausted", "session_trust_read_timeout", "session_trust_read_pending", "session_trust_revoked", "session_trust_peer_missing", "session_trust_store_missing"} {
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"`+code+`"}`+"\n"))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code {
			t.Fatal(code)
		}
	}
}
