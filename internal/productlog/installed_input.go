package productlog

func init() {
	for _, stage := range []string{"client_lock", "client_authority", "client_write", "client_response", "worker_authority", "worker_scope", "worker_apply"} {
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["installed_input_timing_"+stage+"_"+bucket] = true
			allowed["installed_input_heartbeat_timing_"+stage+"_"+bucket] = true
		}
	}
	for _, code := range []string{"installed_input_selected", "installed_input_lease_ready", "installed_input_connect_failed", "installed_input_heartbeat_failed"} {
		allowed[code] = true
	}
	for _, stage := range []string{"lease_metadata", "pipe_open", "service_pid", "service_manager", "service_open", "service_config", "service_status", "service_process_open", "service_process_metadata", "service_image", "service_image_acl", "lease_deadline", "lease_write", "worker_ready", "client_validation", "unknown"} {
		allowed["installed_input_stage_"+stage] = true
	}
	for _, number := range []string{"2", "5", "32", "231", "1314", "other", "unknown"} {
		allowed["installed_input_native_"+number] = true
	}
}
