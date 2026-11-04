package handoff

// Submit includes deadline setup, the pipe write lock and app scheduling up to
// WriteFile entry. Receive includes pipe transfer, worker reader scheduling and
// decoding. Neither span proves time spent inside the Windows kernel.
// Completion can precede OR follow receipt; do not add it to request delivery.
type CaptureRequest struct {
	Measured              bool   `json:"measured"`
	SubmitMicros          uint32 `json:"submitMicros"`
	ReceiveMicros         uint32 `json:"receiveMicros"`
	WriteCompletionMicros uint32 `json:"writeCompletionMicros"`
}

func (p CaptureRequest) valid(delivery uint32) bool {
	if !p.Measured {
		return p == (CaptureRequest{})
	}
	return uint64(p.SubmitMicros)+uint64(p.ReceiveMicros) == uint64(delivery) && p.WriteCompletionMicros <= uint32(Window.Microseconds())
}

// Missing, reversed and out-of-window observations discard only this optional
// detail. In particular a reader can run before the writer returns from IO.
func CaptureRequestFromClocks(sent, submitted, returned, received uint64) CaptureRequest {
	if sent == 0 || submitted < sent || returned < submitted || received < submitted || returned-sent > uint64(Window.Microseconds()) || received-sent > uint64(Window.Microseconds()) {
		return CaptureRequest{}
	}
	return CaptureRequest{true, uint32(submitted - sent), uint32(received - submitted), uint32(returned - submitted)}
}
