package productlog

func init() {
	allowed["installed_source_connect_failed"] = true
	for _, stage := range []string{"deadline", "write"} {
		allowed["installed_source_io_stage_"+stage] = true
	}
	for _, reason := range []string{"timeout", "canceled", "closed", "short_write", "native", "other"} {
		allowed["installed_source_io_"+reason] = true
	}
	for _, op := range []string{"start", "admit", "activate", "deactivate", "heartbeat", "stop", "cursor_prepare", "cursor_show", "cursor_restore", "unknown"} {
		allowed["installed_source_command_failed_"+op] = true
	}
	for _, outcome := range []string{"received", "cancelled", "timeout", "reader_ended", "protocol", "rejected"} {
		allowed["installed_source_response_"+outcome] = true
	}
	for _, stage := range []string{"context", "authority", "write", "response", "protocol"} {
		allowed["installed_source_failure_"+stage] = true
	}
	allowed["installed_source_heartbeat_deferred"] = true
	allowed["installed_source_heartbeat_preflight_expired"] = true
	allowed["installed_source_command_preflight_expired"] = true
	allowed["installed_source_heartbeat_failed"] = true
	for _, target := range []string{"default", "winlogon", "limit"} {
		allowed["installed_input_desktop_"+target] = true
	}
	for _, stage := range []string{"pipe_connect", "service_launch", "service_child_connect", "service_bootstrap_ready", "worker_installed_validation", "worker_identity", "worker_service_auth", "worker_agent_auth", "worker_desktop_ready"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["installed_return_cursor_startup_"+stage+"_"+bucket] = true
		}
	}
	allowed["installed_return_cursor_cancelled"] = true
	for _, stage := range []string{"prepare", "show", "restore"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["installed_return_cursor_capture_"+stage+"_"+bucket] = true
		}
	}
	for _, code := range []string{"installed_return_cursor_route_capture", "installed_return_cursor_capture_queued", "installed_return_cursor_capture_failed"} {
		allowed[code] = true
	}
	for _, stage := range []string{"admission", "cursor_open", "cursor_retire"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["installed_input_handoff_timing_"+stage+"_"+bucket] = true
		}
	}
	for _, code := range []string{"installed_return_cursor_failed", "installed_return_cursor_ready", "installed_return_cursor_close_failed", "installed_return_cursor_route_native", "installed_return_cursor_route_protected"} {
		allowed[code] = true
	}
	for _, code := range []string{"installed_input_failure_ack_unconfirmed", "installed_input_failure_before_apply", "installed_input_failure_after_dispatch", "installed_input_failure_stage_unknown"} {
		allowed[code] = true
	}
	for _, stage := range []string{"scope_admission", "apply", "cursor_readback", "scope_completion", "ack_mismatch", "ack_timeout", "ack_eof", "ack_closed", "ack_unclassified"} {
		allowed["installed_input_failure_"+stage] = true
	}
	for _, code := range []string{"installed_source_refresh_requested", "installed_source_refresh_joined", "installed_source_refresh_failed", "installed_source_refresh_active_owner", "installed_source_refresh_capture_busy", "installed_source_refresh_unavailable", "installed_source_capture_ready", "installed_source_capture_protected_ready", "boundary_pressed_native_only", "boundary_pressed_hook_only", "boundary_pressed_native_and_hook"} {
		allowed[code] = true
	}
	for _, stage := range []string{"client_lock", "client_authority", "client_write", "client_response", "worker_authority", "worker_scope", "worker_apply"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["installed_input_timing_"+stage+"_"+bucket] = true
			allowed["installed_input_heartbeat_timing_"+stage+"_"+bucket] = true
			if stage == "client_lock" || stage == "client_authority" || stage == "client_write" || stage == "client_response" {
				allowed["installed_source_timing_"+stage+"_"+bucket] = true
				allowed["installed_source_heartbeat_timing_"+stage+"_"+bucket] = true
			}
		}
	}
	for _, code := range []string{"installed_input_selected", "installed_input_lease_ready", "installed_input_connect_failed", "installed_input_heartbeat_failed", "installed_input_preparation_acknowledged", "installed_input_preparation_missed", "installed_source_receive_only"} {
		allowed[code] = true
	}
	for _, stage := range []string{"lease_metadata", "pipe_open", "service_pid", "service_manager", "service_open", "service_config", "service_status", "service_process_open", "service_process_metadata", "service_image", "service_image_acl", "lease_deadline", "lease_write", "worker_ready", "client_validation", "unknown"} {
		allowed["installed_input_stage_"+stage] = true
		allowed["installed_source_stage_"+stage] = true
	}
	for _, number := range []string{"2", "5", "32", "231", "1314", "other", "unknown"} {
		allowed["installed_input_native_"+number] = true
		allowed["installed_source_native_"+number] = true
	}
}
