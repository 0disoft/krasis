package handoff

import "time"

// ServiceReadinessStage separates service-local work around bootstrap. These
// intervals can overlap agent ReadyWait and must not be added to it. In
// particular, LeaseReceive includes waiting for the agent's request bytes.
type ServiceReadinessStage uint8

const (
	ServiceAgentAuthentication ServiceReadinessStage = iota
	ServiceLoginAdmission
	ServiceLeaseReceive
	ServiceBootstrapValidation
	ServiceReadyAuthorization
	ServiceReadinessStageCount
)

type ServiceReadinessProfile struct {
	Micros   [ServiceReadinessStageCount]uint32 `json:"micros"`
	Measured uint8                              `json:"measured"`
}

func (p ServiceReadinessProfile) Valid() bool {
	if p.Measured>>ServiceReadinessStageCount != 0 {
		return false
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || p.Measured&(1<<i) == 0 && n != 0 {
			return false
		}
	}
	return true
}

func (p *ServiceReadinessProfile) Record(stage ServiceReadinessStage, elapsed time.Duration) {
	if p == nil || stage >= ServiceReadinessStageCount {
		return
	}
	elapsed = max(0, min(elapsed, Window))
	p.Micros[stage] = uint32(elapsed.Microseconds())
	p.Measured |= 1 << stage
}
