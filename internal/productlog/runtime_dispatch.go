package productlog

// DispatchSpan preserves one completed slow dispatch, never independent stage
// maxima. Offsets are monotonic since this agent runner started. Completion is
// not proof of success; session failure records remain authoritative.
type DispatchSpan struct {
	Kind                   string `json:"kind"`
	StartedMicros          int64  `json:"startedMicros"`
	DurationMicros         int64  `json:"durationMicros"`
	ReadToStartMicros      int64  `json:"readToStartMicros,omitempty"`
	ReadMeasured           bool   `json:"readMeasured,omitempty"`
	CoordinatorWaitMicros  int64  `json:"coordinatorWaitMicros,omitempty"`
	AfterCoordinatorMicros int64  `json:"afterCoordinatorMicros,omitempty"`
	CoordinatorMeasured    bool   `json:"coordinatorMeasured,omitempty"`
	InjectionMicros        int64  `json:"injectionMicros,omitempty"`
	InjectionMaxMicros     int64  `json:"injectionMaxMicros,omitempty"`
	Injections             uint64 `json:"injections,omitempty"`
}

// Fixed positions keep record size and classification independent of input.
func DispatchKindIndex(kind string) int {
	switch kind {
	case "control":
		return 0
	case "input":
		return 1
	case "heartbeat":
		return 2
	case "disconnect":
		return 3
	case "unknown":
		return 4
	}
	return -1
}

func (s DispatchSpan) valid() bool {
	if DispatchKindIndex(s.Kind) < 0 || s.StartedMicros < 1 || s.DurationMicros < 50_000 || s.ReadToStartMicros < 0 ||
		s.CoordinatorWaitMicros < 0 || s.AfterCoordinatorMicros < 0 || s.InjectionMicros < 0 || s.InjectionMaxMicros < 0 {
		return false
	}
	if !s.ReadMeasured && s.ReadToStartMicros != 0 || s.ReadMeasured && s.ReadToStartMicros >= s.StartedMicros {
		return false
	}
	if !s.CoordinatorMeasured && (s.CoordinatorWaitMicros != 0 || s.AfterCoordinatorMicros != 0) ||
		s.CoordinatorMeasured && (s.Kind != "input" && s.Kind != "heartbeat" ||
			s.CoordinatorWaitMicros > s.DurationMicros || s.AfterCoordinatorMicros > s.DurationMicros-s.CoordinatorWaitMicros) {
		return false
	}
	return s.InjectionMicros <= s.DurationMicros && s.InjectionMaxMicros <= s.InjectionMicros &&
		(s.Injections != 0 || s.InjectionMicros == 0 && s.InjectionMaxMicros == 0)
}
