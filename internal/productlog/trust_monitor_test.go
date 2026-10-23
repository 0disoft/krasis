package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProductMonitorReasonsSurviveExport(t *testing.T) {
	for _, code := range []string{"session_certificate_deadline", "session_guard_exited", "session_guard_close_failed", "session_trust_check_failed", "session_trust_busy_exhausted", "session_trust_read_timeout", "session_trust_read_pending", "session_trust_read_deferred", "session_trust_read_recovered", "session_trust_freshness_expired", "session_trust_revoked", "session_trust_peer_missing", "session_trust_store_missing", "protected_input_detected"} {
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"`+code+`"}`+"\n"))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code {
			t.Fatal(code)
		}
	}
}

func TestTrustTimingCodesSurviveExportWithoutArbitraryDetails(t *testing.T) {
	for _, code := range []string{"trust_timing_prelock_lt1ms", "trust_timing_prelock_parent_acl_ge1s", "trust_timing_lock_lt100ms", "trust_timing_store_read_lt1s", "trust_timing_store_read_acl_ge1s", "trust_timing_certificate_ge1s", "trust_timing_retry_wait_lt100ms", "trust_active_prelock", "trust_active_prelock_parent_acl_query", "trust_active_prelock_lockfile_open", "trust_active_store_read_acl"} {
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

func TestProtectedInputReasonsSurviveExportWithoutPrivateDetails(t *testing.T) {
	for _, code := range []string{"protected_input_desktop_unavailable", "protected_input_non_default_desktop", "protected_input_credential_broker", "protected_input_identity_unknown", "protected_input_inspection_failed", "protected_inspection_reconnecting"} {
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"`+code+`","path":"private","title":"private"}`+"\n"))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code {
			t.Fatal(code)
		}
		encoded, err := json.Marshal(result)
		if err != nil || strings.Contains(string(encoded), "private") {
			t.Fatal("private details crossed support export")
		}
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-09-17T00:00:00Z","code":"protected_input_private_detail"}`+"\n"))
	if !result.Incomplete || len(result.Events) != 0 {
		t.Fatal("arbitrary protection detail crossed support export")
	}
}

func TestTrustExpiryObservationExportsOnlyClosedStates(t *testing.T) {
	for _, state := range []string{"idle", "pending", "not_started", "finished_unpublished", "reading", "result_ok_before_deadline", "result_ok_late", "result_error_before_deadline", "result_error_late", "late_lt1ms", "late_lt10ms", "late_lt100ms", "late_lt1s", "late_ge1s"} {
		code := "trust_expiry_" + state
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-15T09:56:25Z","code":"`+code+`","error":"private certificate path"}`+"\n"))
		data, _ := json.Marshal(result)
		if result.Incomplete || len(result.Events) != 1 || strings.Contains(string(data), "private") {
			t.Fatal("trust expiry evidence lost or leaked details", code)
		}
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader("{\"time\":\"2026-10-15T09:56:25Z\",\"code\":\"trust_expiry_private_error\"}\n"))
	if !result.Incomplete || len(result.Events) != 0 {
		t.Fatal("unbounded trust expiry state exported")
	}
}

func TestCertificateSubstepsExportOnlyFixedTimingCodes(t *testing.T) {
	for _, phase := range []string{"open", "acl", "bytes", "close", "validate", "match"} {
		code := "trust_timing_certificate_" + phase + "_ge1s"
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-15T09:56:25Z","code":"`+code+`","certificate":"private"}`+"\n"))
		encoded, _ := json.Marshal(result)
		if result.Incomplete || len(result.Events) != 1 || strings.Contains(string(encoded), "private") {
			t.Fatal("certificate timing lost or data escaped", phase)
		}
	}
}

func TestLockFileDetailsExportOnlyFixedProgressCodes(t *testing.T) {
	for _, detail := range []string{"info", "owner", "acl_query", "acl_check"} {
		code := "trust_active_prelock_lockfile_" + detail
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-19T12:24:48Z","code":"`+code+`","path":"private","sid":"private"}`+"\n"))
		encoded, _ := json.Marshal(result)
		if result.Incomplete || len(result.Events) != 1 || strings.Contains(string(encoded), "private") {
			t.Fatal("lock file evidence lost or data escaped", detail)
		}
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader("{\"time\":\"2026-10-19T12:24:48Z\",\"code\":\"trust_active_prelock_lockfile_private\"}\n"))
	if !result.Incomplete || len(result.Events) != 0 {
		t.Fatal("unknown lock file detail exported")
	}
}

func TestStoreOpenDetailsExportOnlyFixedProgressCodes(t *testing.T) {
	for _, detail := range []string{"native", "info", "wrap"} {
		code := "trust_active_store_read_open_" + detail
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-23T00:00:00Z","code":"`+code+`","path":"private","error":"private"}`+"\n"))
		encoded, _ := json.Marshal(result)
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code || strings.Contains(string(encoded), "private") {
			t.Fatal("store open detail lost or private data exported", detail)
		}
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader("{\"time\":\"2026-10-23T00:00:00Z\",\"code\":\"trust_active_store_read_open_private\"}\n"))
	if !result.Incomplete || len(result.Events) != 0 {
		t.Fatal("unknown store open detail exported")
	}
}
