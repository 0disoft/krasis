package productlog

// OS counters span adjacent sampler observations, not an exact GC pause.
// PageFaults includes soft faults. System times describe the processor group
// returned by Windows, not necessarily every group on hosts with >64 CPUs.
type GCOSWindow struct {
	ReadMicros        uint64 `json:"readMicros"`
	CPUMicros         uint64 `json:"cpuMicros"`
	CPUKnown          bool   `json:"cpuKnown"`
	PageFaults        uint64 `json:"pageFaults"`
	FaultsKnown       bool   `json:"faultsKnown"`
	SystemBusyMicros  uint64 `json:"systemBusyMicros"`
	SystemTotalMicros uint64 `json:"systemTotalMicros"`
	SystemKnown       bool   `json:"systemKnown"`
}

func (w GCOSWindow) valid() bool {
	return (w.CPUKnown || w.CPUMicros == 0) &&
		(w.FaultsKnown || w.PageFaults == 0) &&
		(w.SystemKnown || w.SystemBusyMicros == 0 && w.SystemTotalMicros == 0) &&
		w.SystemBusyMicros <= w.SystemTotalMicros
}
