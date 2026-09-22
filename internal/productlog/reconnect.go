package productlog

import "strings"

// Reconnect reasons are a closed vocabulary. Never log arbitrary descriptor
// strings: observers and wrapped errors can contain private peer details.
var reconnectReasons = map[[3]string]string{
	{"source-capture", "capture-runtime", "unregister_failed"}:         "reconnect_capture_unregister_failed",
	{"source-capture", "capture-runtime", "command_timeout"}:           "reconnect_capture_command_timeout",
	{"source-capture", "capture-runtime", "foreground_lost"}:           "reconnect_capture_foreground_lost",
	{"liveness", "heartbeat", "peer_timeout"}:                          "reconnect_heartbeat_timeout",
	{"peer-reader", "read", "remote_eof"}:                              "reconnect_remote_eof",
	{"peer-reader", "read", "after_local_interrupt"}:                   "reconnect_after_local_interrupt",
	{"peer-reader", "read", "frame_truncated"}:                         "reconnect_frame_truncated",
	{"peer-reader", "read", "io_failure"}:                              "reconnect_read_io_failure",
	{"peer-writer", "request-write", "outcome_unknown"}:                "reconnect_request_write_failed",
	{"peer-writer", "grant-write", "outcome_unknown"}:                  "reconnect_grant_write_failed",
	{"peer-writer", "revoke-write", "outcome_unknown"}:                 "reconnect_revoke_write_failed",
	{"peer-dispatch", "control", "failed"}:                             "reconnect_control_dispatch_failed",
	{"peer-dispatch", "input", "failed"}:                               "reconnect_input_dispatch_failed",
	{"peer-dispatch", "heartbeat", "failed"}:                           "reconnect_heartbeat_dispatch_failed",
	{"outbound-control", "grant-apply", "state_transition_failed"}:     "reconnect_grant_apply_failed",
	{"boundary-admission", "request-begin", "state_transition_failed"}: "reconnect_boundary_begin_failed",
	{"source-session", "runtime", "connection_interrupted"}:            "reconnect_source_interrupted",
	{"source-session", "source-activate", "activation_failed"}:         "reconnect_source_activation_failed",
	{"source-capture", "focus-release", "capture_retained"}:            "reconnect_capture_retained",
}

func init() {
	for _, phase := range []string{"admission", "io", "deadline"} {
		for _, code := range []string{"failed", "timeout", "canceled"} {
			reconnectReasons[[3]string{"peer-writer", "write-" + phase, code}] = "reconnect_write_" + phase + "_" + code
		}
	}
	for _, phase := range []string{"source-close", "connection-close", "nonlocal-state", "unexpected-stop", "nil-connection"} {
		reconnectReasons[[3]string{"supervisor", phase, "failed"}] = "reconnect_supervisor_" + strings.ReplaceAll(phase, "-", "_") + "_failed"
	}
	reconnectReasons[[3]string{"recovery", "deadline", "exceeded"}] = "reconnect_recovery_deadline_exceeded"
	for _, stage := range []string{"low-level-hook", "raw-mouse", "wheel-capture", "thread-join", "hook-install", "hook-release", "raw-register", "raw-release", "raw-read", "raw-buffer", "raw-route", "message-loop", "pointer-snapshot"} {
		reconnectReasons[[3]string{"source-hook", stage, "failed"}] = "reconnect_source_" + strings.ReplaceAll(stage, "-", "_") + "_failed"
	}
	for _, phase := range []string{"capture-activate", "focus-restore"} {
		for _, code := range []string{"thread_detach_failed", "thread_attach_failed", "verification_lost", "api_rejected", "setup_failed", "activation_failed", "foreground_unavailable", "foreground_thread_unavailable", "foreground_probe_failed", "rollback_failed"} {
			prefix := "reconnect_capture_"
			if phase == "focus-restore" {
				prefix = "reconnect_focus_restore_"
			}
			reconnectReasons[[3]string{"source-capture", phase, code}] = prefix + code
		}
	}
	for _, reason := range reconnectReasons {
		allowed[reason] = true
	}
	for _, code := range []string{"reconnect_acquire_failed", "reconnect_session_unknown", "heartbeat_timeout_observed", "disconnect_requested", "reconnect_source_hook_failed", "reconnect_return_controller_failed", "reconnect_boundary_orchestrator_failed", "reconnect_peer_session_failed", "reconnect_peer_liveness_failed"} {
		allowed[code] = true
	}
	var faults []string
	for code := range allowed {
		if strings.HasPrefix(code, "reconnect_") {
			faults = append(faults, "fault_"+strings.TrimPrefix(code, "reconnect_"))
			faults = append(faults, "activation_"+strings.TrimPrefix(code, "reconnect_"))
		}
	}
	for _, code := range faults {
		allowed[code] = true
	}
}

func FaultCode(component, phase, code string) string {
	return "fault_" + strings.TrimPrefix(ReconnectCode("session", component, phase, code), "reconnect_")
}

func ActivationCode(component, phase, code string) string {
	return "activation_" + strings.TrimPrefix(ReconnectCode("session", component, phase, code), "reconnect_")
}

func ReconnectCode(kind, component, phase, code string) string {
	if kind == "acquire" {
		return "reconnect_acquire_failed"
	}
	if kind != "session" {
		return "reconnect_session_unknown"
	}
	if reason, ok := reconnectReasons[[3]string{component, phase, code}]; ok {
		return reason
	}
	if phase == "runtime" && code == "component_failed" {
		switch component {
		case "source-hook":
			return "reconnect_source_hook_failed"
		case "return-controller":
			return "reconnect_return_controller_failed"
		case "boundary-orchestrator":
			return "reconnect_boundary_orchestrator_failed"
		case "peer-session":
			return "reconnect_peer_session_failed"
		case "peer-liveness":
			return "reconnect_peer_liveness_failed"
		}
	}
	return "reconnect_session_unknown"
}
