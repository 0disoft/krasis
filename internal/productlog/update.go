package productlog

func init() {
	for _, code := range []string{"peer_update_permission_saved", "peer_update_queued", "peer_update_receive_failed", "peer_update_ready", "peer_update_apply_started", "peer_update_apply_failed", "peer_update_agent_verified"} {
		allowed[code] = true
	}
}
