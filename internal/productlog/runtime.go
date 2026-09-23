package productlog

// RuntimeSummary contains process-level counters and approximate worker progress,
// never input contents. Duration maxima are for this sampling interval; GC and
// runnable histograms use upper-bound buckets <=100us,1ms,10ms,100ms,1s,>1s.
// An idle socket can have a large read age without being faulty.
type RuntimeSummary struct {
	HookCalls                 uint64    `json:"hookCalls"`
	RawInputs                 uint64    `json:"rawInputs"`
	QueueHighWatermark        uint64    `json:"queueHighWatermark"`
	QueueEnqueued             uint64    `json:"queueEnqueued"`
	QueueDequeued             uint64    `json:"queueDequeued"`
	QueueCoalesced            uint64    `json:"queueCoalesced"`
	QueueStaleDiscards        uint64    `json:"queueStaleDiscards"`
	QueueMaxResidenceMicros   int64     `json:"queueMaxResidenceMicros"`
	FrameReadBytes            uint64    `json:"frameReadBytes"`
	LastReadByteAgeMicros     int64     `json:"lastReadByteAgeMicros"`
	ReadByteObserved          bool      `json:"readByteObserved"`
	HookMaxMicros             int64     `json:"hookMaxMicros"`
	HookActiveMicros          int64     `json:"hookActiveMicros"`
	LastHookAgeMicros         int64     `json:"lastHookAgeMicros"`
	HookObserved              bool      `json:"hookObserved"`
	IntervalMicros            int64     `json:"intervalMicros"`
	HeapBytes                 uint64    `json:"heapBytes"`
	AllocatedBytes            uint64    `json:"allocatedBytes"`
	GCPauses                  [6]uint64 `json:"gcPauses"`
	RunnableDelays            [6]uint64 `json:"runnableDelays"`
	WorkingSetBytes           uint64    `json:"workingSetBytes"`
	WorkingSetKnown           bool      `json:"workingSetKnown"`
	CPUMicros                 uint64    `json:"cpuMicros"`
	CPUKnown                  bool      `json:"cpuKnown"`
	TimerLateMicros           int64     `json:"timerLateMicros"`
	ReadAgeMicros             int64     `json:"readAgeMicros"`
	DispatchAgeMicros         int64     `json:"dispatchAgeMicros"`
	DispatchKind              string    `json:"dispatchKind"`
	FramesReceived            uint64    `json:"framesReceived"`
	Dispatches                uint64    `json:"dispatches"`
	DispatchMaxMicros         int64     `json:"dispatchMaxMicros"`
	HeartbeatSendAgeMicros    int64     `json:"heartbeatSendAgeMicros"`
	HeartbeatReceiveAgeMicros int64     `json:"heartbeatReceiveAgeMicros"`
	HeartbeatSent             bool      `json:"heartbeatSent"`
	HeartbeatReceived         bool      `json:"heartbeatReceived"`
}

func init() { allowed["runtime_summary"] = true }
func (s RuntimeSummary) valid() bool {
	if s.HookMaxMicros < 0 || s.HookActiveMicros < 0 || s.LastHookAgeMicros < 0 || s.QueueMaxResidenceMicros < 0 {
		return false
	}
	if s.IntervalMicros <= 0 || s.TimerLateMicros < 0 || s.ReadAgeMicros < 0 || s.LastReadByteAgeMicros < 0 || s.DispatchAgeMicros < 0 || s.DispatchMaxMicros < 0 || s.HeartbeatSendAgeMicros < 0 || s.HeartbeatReceiveAgeMicros < 0 {
		return false
	}
	if !s.WorkingSetKnown && s.WorkingSetBytes != 0 || !s.CPUKnown && s.CPUMicros != 0 {
		return false
	}
	switch s.DispatchKind {
	case "idle", "control", "input", "heartbeat", "disconnect", "unknown":
		return true
	}
	return false
}
func (l *Logger) Runtime(s RuntimeSummary) {
	if s.valid() {
		l.enqueueRecord(record{Code: "runtime_summary", Runtime: &s})
	}
}
