package residentagent

import "github.com/0disoft/krasis/internal/handoff"

// Called after the handoff summary is emitted, outside active input/cleanup.
// A fast network exchange can still contain a slow local capture transition.
// Only measured spans qualify; they are nested and must never be added.
func handoffNeedsTrace(s handoff.Summary) bool {
	if s.Role != "source" {
		return false
	}
	if s.Exchange != nil && s.Exchange.Measured&1 != 0 &&
		s.Exchange.Micros[handoff.RequestWrittenToGrantRead] >= 80000 {
		return true
	}
	for _, stage := range [...]handoff.Stage{handoff.CaptureActivate, handoff.SourceDeactivate} {
		if s.Preparation.Measured&(uint64(1)<<stage) != 0 && s.Preparation.Micros[stage] >= 80000 {
			return true
		}
	}
	return false
}
