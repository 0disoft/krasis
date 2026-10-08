package productlog

func init() {
	allowed["release_check_cancelled"] = true
	for _, code := range []string{"peer_update_permission_saved", "peer_update_queued", "peer_update_receive_failed", "peer_update_ready", "peer_update_apply_started", "peer_update_apply_failed", "peer_update_agent_verified", "peer_update_apply_preflight_failed", "peer_update_apply_busy", "peer_update_apply_session_changed", "peer_update_apply_claim_failed"} {
		allowed[code] = true
	}
}
