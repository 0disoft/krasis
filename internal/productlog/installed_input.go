package productlog

func init() {
	for _, code := range []string{"installed_input_failure_ack_unconfirmed", "installed_input_failure_before_apply", "installed_input_failure_after_dispatch", "installed_input_failure_stage_unknown"} {
		allowed[code] = true
	}
	for _, code := range []string{"installed_source_refresh_requested", "installed_source_refresh_joined", "installed_source_refresh_failed", "installed_source_capture_ready", "boundary_pressed_native_only", "boundary_pressed_hook_only", "boundary_pressed_native_and_hook"} {
		allowed[code] = true
	}
	for _, stage := range []string{"client_lock", "client_authority", "client_write", "client_response", "worker_authority", "worker_scope", "worker_apply"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["installed_input_timing_"+stage+"_"+bucket] = true
			allowed["installed_input_heartbeat_timing_"+stage+"_"+bucket] = true
		}
	}
	for _, code := range []string{"installed_input_selected", "installed_input_lease_ready", "installed_input_connect_failed", "installed_input_heartbeat_failed", "installed_input_preparation_acknowledged", "installed_input_preparation_missed", "installed_source_receive_only"} {
		allowed[code] = true
	}
	for _, stage := range []string{"lease_metadata", "pipe_open", "service_pid", "service_manager", "service_open", "service_config", "service_status", "service_process_open", "service_process_metadata", "service_image", "service_image_acl", "lease_deadline", "lease_write", "worker_ready", "client_validation", "unknown"} {
		allowed["installed_input_stage_"+stage] = true
	}
	for _, number := range []string{"2", "5", "32", "231", "1314", "other", "unknown"} {
		allowed["installed_input_native_"+number] = true
	}
}
