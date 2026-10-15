package productlog

import (
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/inputworker"
)

func TestInputConnectionDiagnosticAllowlistRejectsDynamicStages(t *testing.T) {
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
	for _, code := range []string{"installed_input_stage_pipe_open", "installed_input_stage_service_config", "installed_input_stage_worker_ready", "installed_input_native_5"} {
		if !allowed[code] {
			t.Fatal("fixed input connection diagnostic omitted")
		}
	}
	if allowed["installed_input_stage_private-path"] || allowed["installed_input_native_123456"] {
		t.Fatal("unbounded input connection diagnostic accepted")
	}
}
