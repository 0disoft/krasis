package productpanel

import "time"

// Owned by the UI thread. The budget is per user-started connection, not per
// poll or brief CONNECTED transition, so repeated faults cannot form a loop.
type AutomaticRecovery struct {
	Alias, Fingerprint string
	Attempts           int
	blocked, diagnosed bool
	uncertain          bool
	next               time.Time
	displaySince       time.Time
	displayReadySince  time.Time
	displayNext        time.Time
	displayAttempts    int
	displayExpired     bool
}

func (r *AutomaticRecovery) Reset() { *r = AutomaticRecovery{} }
func (r *AutomaticRecovery) Stop()  { *r = AutomaticRecovery{blocked: true} }

func (r *AutomaticRecovery) Next(now time.Time, view View) string {
	if r.blocked {
		return ""
	}
	if r.uncertain {
		if view.Fingerprints[r.Alias] != r.Fingerprint || view.State == "CONNECTED" && view.PeerAlias != r.Alias {
			r.Stop()
			return "stale_peer"
		}
		if view.State != "CONNECTED" || view.PeerAlias != r.Alias {
			return "outcome_unknown"
		}
		r.uncertain = false // Only the pinned peer's connected state resolves it.
	}
	if view.State == "CONNECTED" {
		if view.PeerAlias != "" && len(view.Fingerprints[view.PeerAlias]) == 64 {
			r.Alias, r.Fingerprint = view.PeerAlias, view.Fingerprints[view.PeerAlias]
		}
		r.diagnosed = false
		r.next = time.Time{}
		r.displaySince = time.Time{}
		r.displayReadySince = time.Time{}
		r.displayNext = time.Time{}
		return ""
	}
	if view.State == "LOCAL" {
		if view.StopReason == "display_change" && r.Alias != "" && view.Fingerprints[r.Alias] == r.Fingerprint {
			if r.displayExpired {
				return ""
			}
			if r.displaySince.IsZero() {
				r.displaySince = now
			}
			if now.Sub(r.displaySince) >= 30*time.Second || r.displayAttempts >= 3 {
				r.displayExpired = true
				return "display_recovery_expired"
			}
			if !view.DisplayReady {
				r.displayReadySince = time.Time{}
				return ""
			}
			if r.displayReadySince.IsZero() {
				r.displayReadySince = now
			}
			if now.Sub(r.displayReadySince) < 3*time.Second || now.Before(r.displayNext) {
				return ""
			}
			r.displayAttempts++
			r.displayNext = now.Add([]time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}[r.displayAttempts-1])
			return "display_reconnect"
		}
		r.Alias, r.Fingerprint = "", ""
		r.diagnosed = false
		r.displaySince = time.Time{}
		r.displayReadySince = time.Time{}
		r.displayNext = time.Time{}
		return ""
	}
	if (view.State != "FAULTED" && view.State != "RECONNECTING") || (view.State == "RECONNECTING" && r.Alias == "") {
		return ""
	}
	if !r.diagnosed {
		r.diagnosed = true
		r.next = now.Add(5 * time.Second)
		return "auto_diagnose"
	}
	// RECONNECTING already has a safe supervisor retry loop. Do not replace it.
	if view.State != "FAULTED" || r.Alias == "" || r.Attempts >= 3 || now.Before(r.next) {
		return ""
	}
	r.Attempts++
	r.next = now.Add([]time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}[r.Attempts-1])
	return "auto_recover"
}

func (r *AutomaticRecovery) Notice(state string) string {
	if state != "FAULTED" || r.blocked || r.Alias == "" {
		return ""
	}
	if r.Attempts >= 3 {
		return "자동 복구 3회 후에도 오류가 남아 시도를 중단했습니다. 진단 저장 결과를 확인하고 수동 재시작 또는 업데이트를 진행하세요."
	}
	return "진단을 자동 저장·전송하고 안전한 에이전트 재시작을 준비합니다. 자동 복구는 최대 3회 시도합니다."
}
