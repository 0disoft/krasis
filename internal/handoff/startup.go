package handoff

import (
	"context"
	"time"
)

// StartupStage spans are local to the service or child, not cross-process
// timestamps. ServiceBootstrapReady contains child work and must not be added
// to it. The existing handoff vector indices and schema remain unchanged.
type StartupStage uint8

const (
	ServiceLaunch StartupStage = iota
	ServiceChildConnect
	ServiceBootstrapReady
	WorkerInstalledValidation
	WorkerIdentity
	WorkerServiceAuth
	WorkerAgentAuth
	WorkerDesktopReady
	StartupStageCount
)

type StartupProfile struct {
	Schema       uint8                     `json:"schema"`
	Micros       [StartupStageCount]uint32 `json:"micros"`
	Measured     uint8                     `json:"measured"`
	LoadMicros   uint32                    `json:"loadMicros,omitempty"`
	LoadMeasured bool                      `json:"loadMeasured,omitempty"`
}

func (p StartupProfile) Valid() bool {
	if p.Schema != 1 {
		return false
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || (p.Measured&(1<<i) == 0 && n != 0) {
			return false
		}
	}
	if p.LoadMicros > uint32(Window.Microseconds()) || (!p.LoadMeasured && p.LoadMicros != 0) ||
		(p.LoadMeasured && (p.Measured&(1<<WorkerInstalledValidation) == 0 || p.LoadMicros > p.Micros[WorkerInstalledValidation])) {
		return false
	}
	return true
}

// RecordLoad is the measured file/metadata load within WorkerInstalledValidation,
// not a new authority signal or an additional contribution to startup elapsed.
func (p *StartupProfile) RecordLoad(elapsed time.Duration) {
	if p == nil {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
	}
	p.LoadMicros = uint32(elapsed.Microseconds())
	p.LoadMeasured = true
}

func (p *StartupProfile) Record(stage StartupStage, elapsed time.Duration) {
	if p == nil || stage >= StartupStageCount {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
	}
	p.Schema = 1
	p.Micros[stage] = uint32(elapsed.Microseconds())
	p.Measured |= 1 << stage
}

// RecordStartup copies only validated diagnostic data into an active installed
// receiver attempt. It never supplies readiness or input authority.
func RecordStartup(ctx context.Context, profile *StartupProfile) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || profile == nil || !profile.Valid() || profile.Measured == 0 {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if a.finished || time.Since(s.collector.started) >= Window || a.summary.Startup != nil || a.summary.Role != "receiver" || a.summary.Backend != "installed" {
		return
	}
	copy := *profile
	a.summary.Startup = &copy
}
