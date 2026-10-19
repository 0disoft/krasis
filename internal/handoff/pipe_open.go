package handoff

import "context"

// PipeOpenProfile partitions the receiver's pre-authentication pipe open.
// It overlaps PipeOpen, not an additional contribution to entry latency.
type PipeOpenProfile struct {
	Schema     uint8  `json:"schema"`
	Attempts   uint32 `json:"attempts"`
	Busy       uint32 `json:"busy"`
	Missing    uint32 `json:"missing"`
	OpenMicros uint32 `json:"openMicros"`
	WaitMicros uint32 `json:"waitMicros"`
}

func (p PipeOpenProfile) Valid() bool {
	return p.Schema == 1 && p.Attempts > 0 && p.Attempts <= 65535 &&
		uint64(p.Busy)+uint64(p.Missing) <= uint64(p.Attempts) &&
		uint64(p.OpenMicros)+uint64(p.WaitMicros) <= uint64(Window.Microseconds())
}

func validPipeOpen(s Summary) bool {
	return s.PipeOpen == nil || s.Role == "receiver" && s.Backend == "installed" &&
		s.Preparation.Measured&(1<<PipeOpen) != 0 && s.PipeOpen.Valid() &&
		uint64(s.PipeOpen.OpenMicros)+uint64(s.PipeOpen.WaitMicros) <= uint64(s.Preparation.Micros[PipeOpen])
}

func RecordPipeOpen(ctx context.Context, profile PipeOpenProfile) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || !profile.Valid() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !exchangeActive(s, a) || a.summary.Role != "receiver" || a.summary.Backend != "installed" || a.summary.PipeOpen != nil {
		return
	}
	a.summary.PipeOpen = &profile
}
