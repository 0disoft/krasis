package handoff

import (
	"context"
	"time"
)

// Adjacent QPC spans on one PC, from request write entry to caller resumption.
// Reply delivery includes the native write, pipe transit and reader scheduling;
// it does not claim to distinguish kernel write completion from reader wakeup.
const CaptureIPCStageCount = 8

type CaptureIPC struct {
	Measured                bool           `json:"measured"`
	RequestDeliveryMicros   uint32         `json:"requestDeliveryMicros"`
	ReadAuthorizationMicros uint32         `json:"readAuthorizationMicros"`
	DispatchMicros          uint32         `json:"dispatchMicros"`
	ActivationMicros        uint32         `json:"activationMicros"`
	ReplyAdmissionMicros    uint32         `json:"replyAdmissionMicros"`
	ReplyEncodeMicros       uint32         `json:"replyEncodeMicros"`
	ReplyDeliveryMicros     uint32         `json:"replyDeliveryMicros"`
	CallerResumeMicros      uint32         `json:"callerResumeMicros"`
	Request                 CaptureRequest `json:"request,omitzero"`
}

func (p CaptureIPC) Valid() bool {
	if !p.Measured {
		return p == (CaptureIPC{})
	}
	var total uint64
	for _, n := range [...]uint32{p.RequestDeliveryMicros, p.ReadAuthorizationMicros, p.DispatchMicros, p.ActivationMicros, p.ReplyAdmissionMicros, p.ReplyEncodeMicros, p.ReplyDeliveryMicros, p.CallerResumeMicros} {
		total += uint64(n)
	}
	return total <= uint64(Window.Microseconds()) && p.Request.valid(p.RequestDeliveryMicros)
}

// Clocks are observations only. Missing/reversed/out-of-window clocks discard
// the entire chain, including an observed zero, without changing command results.
func CaptureIPCFromClocks(clocks [CaptureIPCStageCount + 1]uint64) CaptureIPC {
	if clocks[0] == 0 || clocks[len(clocks)-1] < clocks[0] || clocks[len(clocks)-1]-clocks[0] > uint64(Window.Microseconds()) {
		return CaptureIPC{}
	}
	var spans [CaptureIPCStageCount]uint32
	for i := range spans {
		if clocks[i+1] < clocks[i] {
			return CaptureIPC{}
		}
		spans[i] = uint32(clocks[i+1] - clocks[i])
	}
	return CaptureIPC{Measured: true, RequestDeliveryMicros: spans[0], ReadAuthorizationMicros: spans[1], DispatchMicros: spans[2], ActivationMicros: spans[3], ReplyAdmissionMicros: spans[4], ReplyEncodeMicros: spans[5], ReplyDeliveryMicros: spans[6], CallerResumeMicros: spans[7]}
}

func RecordCaptureIPC(ctx context.Context, ipc CaptureIPC) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || !ipc.Measured || !ipc.Valid() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if a.finished || s.collector.stopped.Load() || time.Since(s.collector.started) >= Window || a.summary.Role != "source" || a.summary.Capture == nil {
		return
	}
	p := a.summary.Capture
	if p.Measured&(1<<CaptureIPCResponse) == 0 || p.IPC.Measured {
		return
	}
	next := *p
	next.IPC = ipc
	if next.Valid() {
		*p = next
	}
}
