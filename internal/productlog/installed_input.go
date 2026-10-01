package productlog

func init() {
	for _, code := range []string{"installed_input_selected", "installed_input_lease_ready", "installed_input_connect_failed", "installed_input_heartbeat_failed"} {
		allowed[code] = true
	}
}
