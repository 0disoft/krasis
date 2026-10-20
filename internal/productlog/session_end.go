package productlog

import "time"

// Closed vocabularies only. No error strings, keys, desktop/window names,
// clipboard contents, endpoint addresses, or credentials cross this boundary.
type SessionEndSummary struct {
	ObservedAt         string `json:"observedAt"`
	Trigger            bool   `json:"trigger"`
	Component          string `json:"component"`
	Fault              string `json:"fault"`
	Canceled           bool   `json:"canceled"`
	Timeout            bool   `json:"timeout"`
	Closed             bool   `json:"closed"`
	NativeError        uint32 `json:"nativeError"`
	NativeKnown        bool   `json:"nativeKnown"`
	NetworkError       string `json:"networkError,omitempty"`
	NetworkPolicy      string `json:"networkPolicy,omitempty"`
	LastLifecycle      string `json:"lastLifecycle"`
	LifecycleAgeMicros int64  `json:"lifecycleAgeMicros"`
	CaptureMode        string `json:"captureMode"`
}

func init() { allowed["session_end_detail"] = true }

func (s SessionEndSummary) valid() bool {
	switch s.NetworkError {
	case "", "not_observed", "network_failed", "network_canceled", "network_access_denied", "network_refused", "network_unreachable", "network_reset", "network_aborted", "network_timeout":
	default:
		return false
	}
	if s.NetworkPolicy != "" && s.NetworkPolicy != "unknown" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, s.ObservedAt); err != nil {
		return false
	}
	if !s.NativeKnown && s.NativeError != 0 {
		return false
	}
	if s.LifecycleAgeMicros < 0 {
		return false
	}
	switch s.LastLifecycle {
	case "unknown":
		if s.LifecycleAgeMicros != 0 {
			return false
		}
	case "suspend", "resume", "session_lock", "session_disconnect", "logoff", "shutdown", "display_change", "desktop_switch", "monitor_failed":
	default:
		return false
	}
	switch s.CaptureMode {
	case "unknown", "normal", "protected", "receive_only":
	default:
		return false
	}
	if len(s.Fault) < 6 || s.Fault[:6] != "fault_" || !allowed[s.Fault] {
		return false
	}
	switch s.Component {
	case "unknown", "source-local-return", "source-session", "source-hook", "boundary-orchestrator", "return-controller", "peer-session", "preparation-sender", "preparation-receiver", "planned-lease-return", "lease-watchdog", "peer-liveness":
		return true
	}
	return false
}

func (l *Logger) SessionEnd(s SessionEndSummary) {
	s.LastLifecycle, s.CaptureMode = "unknown", "unknown"
	s.LifecycleAgeMicros = 0
	if s.valid() {
		l.enqueueRecord(record{Code: "session_end_detail", SessionEnd: &s})
	}
}
