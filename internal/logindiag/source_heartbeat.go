package logindiag

// Fixed local diagnostics only. These states never grant authority or prove
// native cleanup. An empty stage preserves reports from older installations.
var SourceHeartbeatStages = [...]string{"", "service_presence", "agent_presence", "reply_gate", "reply_presence", "reply_write", "replied"}

func (v SourceExit) validHeartbeat() bool {
	valid := func(stage string) bool {
		for _, candidate := range SourceHeartbeatStages {
			if stage == candidate {
				return true
			}
		}
		return false
	}
	if !valid(v.HeartbeatStage) || !valid(v.HeartbeatSlowStage) || v.HeartbeatSlowBucket > 5 {
		return false
	}
	return (v.HeartbeatSlowStage == "") == (v.HeartbeatSlowBucket == 0) &&
		(v.HeartbeatStage != "" || v.HeartbeatSlowStage == "") && v.HeartbeatSlowStage != "replied"
}
