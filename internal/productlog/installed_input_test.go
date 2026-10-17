package productlog

import (
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/inputworker"
)

func TestInputConnectionDiagnosticAllowlistRejectsDynamicStages(t *testing.T) {
	for _, family := range []struct {
		prefix string
		values []string
	}{
		{"installed_source_command_failed_", []string{"start", "admit", "activate", "deactivate", "heartbeat", "stop", "cursor_prepare", "cursor_show", "cursor_restore", "unknown"}},
		{"installed_source_response_", []string{"received", "cancelled", "timeout", "reader_ended", "protocol", "rejected"}},
		{"installed_source_io_", []string{"timeout", "canceled", "closed", "short_write", "native", "other", "stage_deadline", "stage_write"}},
		{"installed_input_admission_blocked_", []string{"cleanup", "stopped", "active", "unconfigured"}},
	} {
		for _, value := range family.values {
			code := family.prefix + value
			log := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-17T01:55:57Z","code":"`+code+`","key":"PRIVATE"}`+"\n"))
			if log.Incomplete || len(log.Events) != 1 || log.Events[0].Code != code {
				t.Fatal("fixed source command/outcome evidence lost", code)
			}
		}
		if allowed[family.prefix+"PRIVATE"] {
			t.Fatal("dynamic source command/outcome admitted")
		}
	}
	for _, code := range []string{"installed_source_timing_client_response_lt1s", "installed_source_heartbeat_timing_client_authority_ge1s", "installed_source_failure_protocol", "installed_source_command_preflight_expired", "installed_input_preparation_joined"} {
		line := `{"time":"2026-10-15T09:40:00Z","code":"` + code + `","error":"PRIVATE"}` + "\n"
		result := sanitizeLog("agent.jsonl", strings.NewReader(line))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code {
			t.Fatal("source timing/phase lost from sanitized export", code)
		}
	}
	if allowed["installed_source_failure_PRIVATE"] || allowed["installed_source_timing_client_response_123ms"] || allowed["installed_source_timing_worker_apply_lt1ms"] {
		t.Fatal("dynamic or unmeasured source timing accepted")
	}
	for _, result := range []string{"deferred", "failed"} {
		line := `{"time":"2026-10-15T09:40:00Z","code":"installed_source_heartbeat_` + result + `","error":"PRIVATE"}` + "\n"
		result := sanitizeLog("agent.jsonl", strings.NewReader(line))
		if result.Incomplete || len(result.Events) != 1 {
			t.Fatal("source heartbeat result dropped")
		}
	}
	for _, target := range []string{"default", "winlogon", "limit"} {
		line := `{"time":"2026-10-15T09:40:00Z","code":"installed_input_desktop_` + target + `","desktopName":"PRIVATE"}` + "\n"
		result := sanitizeLog("agent.jsonl", strings.NewReader(line))
		if result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != "installed_input_desktop_"+target {
			t.Fatal("fixed desktop class lost", target)
		}
	}
	if allowed["installed_input_desktop_PRIVATE"] {
		t.Fatal("arbitrary desktop name admitted")
	}
	for _, reason := range []string{"active_owner", "capture_busy", "unavailable"} {
		line := `{"time":"2026-10-15T08:37:44Z","code":"installed_source_refresh_` + reason + `","key":"private"}` + "\n"
		result := sanitizeLog("agent.jsonl", strings.NewReader(line))
		if len(result.Events) != 1 || result.Events[0].Code != "installed_source_refresh_"+reason {
			t.Fatal("refresh refusal lost from exported diagnostics", reason)
		}
	}
	if !allowed["installed_return_cursor_cancelled"] || !allowed["installed_return_cursor_startup_service_launch_lt1s"] || !allowed["installed_return_cursor_startup_worker_desktop_ready_lt1ms"] {
		t.Fatal("cursor startup evidence missing from sanitized export")
	}
	if allowed["installed_return_cursor_startup_private-path_lt1ms"] || allowed["installed_return_cursor_startup_service_launch_123ms"] {
		t.Fatal("unbounded cursor startup evidence accepted")
	}
	for _, stage := range []string{"admission", "cursor_open", "cursor_retire"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			if !allowed["installed_input_handoff_timing_"+stage+"_"+bucket] {
				t.Fatal("handoff timing missing from export")
			}
		}
	}
	if allowed["installed_input_handoff_timing_private-path_lt1ms"] || allowed["installed_input_handoff_timing_admission_123ms"] {
		t.Fatal("dynamic handoff timing accepted")
	}
	for _, stage := range []string{"client_lock", "client_authority", "client_write", "client_response", "worker_authority", "worker_scope", "worker_apply"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			if !allowed["installed_input_timing_"+stage+"_"+bucket] || !allowed["installed_input_heartbeat_timing_"+stage+"_"+bucket] {
				t.Fatal("fixed latency diagnostic would be dropped")
			}
		}
	}
	if allowed["installed_input_timing_private-path_lt1ms"] || allowed["installed_input_timing_worker_apply_123ms"] || allowed["installed_input_heartbeat_timing_private-path_lt1ms"] {
		t.Fatal("dynamic latency data accepted")
	}
	if !allowed[inputworker.ConnectionFailureEvent(nil)] || !allowed[inputworker.ConnectionNativeFailureEvent(nil)] {
		t.Fatal("unknown input connection classification would be dropped")
	}
	for _, code := range []string{"installed_input_stage_pipe_open", "installed_input_stage_service_config", "installed_input_stage_worker_ready", "installed_input_native_5", "installed_source_connect_failed", "installed_source_stage_pipe_open", "installed_source_stage_service_config", "installed_source_native_2"} {
		if !allowed[code] {
			t.Fatal("fixed input connection diagnostic omitted")
		}
	}
	if allowed["installed_input_stage_private-path"] || allowed["installed_input_native_123456"] || allowed["installed_source_stage_private-path"] || allowed["installed_source_native_123456"] {
		t.Fatal("unbounded input connection diagnostic accepted")
	}
}
