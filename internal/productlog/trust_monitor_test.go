package productlog

import (
	"strings"
	"testing"
)

func TestProductMonitorReasonsSurviveExport(t *testing.T) {
	for _, code := range []string{"session_certificate_deadline", "session_guard_exited", "session_guard_close_failed", "session_trust_check_failed", "session_trust_busy_exhausted", "session_trust_read_timeout", "session_trust_read_pending", "session_trust_revoked", "session_trust_peer_missing", "session_trust_store_missing", "protected_input_detected"} {
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"`+code+`"}`+"\n"))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code {
			t.Fatal(code)
		}
	}
}

func TestTrustTimingCodesSurviveExportWithoutArbitraryDetails(t *testing.T) {
	for _, code := range []string{"trust_timing_prelock_lt1ms", "trust_timing_prelock_parent_acl_ge1s", "trust_timing_lock_lt100ms", "trust_timing_store_read_lt1s", "trust_timing_store_read_acl_ge1s", "trust_timing_certificate_ge1s", "trust_timing_retry_wait_lt100ms", "trust_active_prelock", "trust_active_prelock_lockfile_open", "trust_active_store_read_acl"} {
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"`+code+`"}`+"\n"))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code {
			t.Fatal(code)
		}
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"trust_timing_private_path"}`+"\n"))
	if !result.Incomplete || len(result.Events) != 0 {
		t.Fatal("arbitrary trust detail crossed support export")
	}
}
