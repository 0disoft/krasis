package handoff

// Native startup spans are child-local and may overlap the journal's startup.
// They are diagnostic durations, never an authority or readiness verdict.
type NativeStartupProfile struct {
	Micros   [5]uint32 `json:"micros"` // installation/self, service, agent, parent, desktop
	Measured uint8     `json:"measured"`
}

func (p NativeStartupProfile) Valid() bool {
	if p.Measured != 0 && p.Measured != 31 {
		return false
	}
	for _, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || p.Measured == 0 && n != 0 {
			return false
		}
	}
	return true
}

func NativeStartupFrom(p *StartupProfile) NativeStartupProfile {
	var result NativeStartupProfile
	if p == nil || !p.Valid() {
		return result
	}
	for i, stage := range [...]StartupStage{WorkerInstalledValidation, WorkerServiceAuth, WorkerAgentAuth, WorkerIdentity, WorkerDesktopReady} {
		if p.Measured&(1<<stage) == 0 {
			return NativeStartupProfile{}
		}
		result.Micros[i] = p.Micros[stage]
	}
	result.Measured = 31
	return result
}
