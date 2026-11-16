package handoff

// Subdivides the existing CallerResumeMicros on the same PC. Publication is
// stamped immediately before the nonblocking channel send; PublishToResume
// includes enqueue scheduling, not just the caller's runnable queue time.
// An opt-in execution trace further separates Go runnable, syscall and waiting
// states. None of these observations is a network-arrival timestamp.
type CaptureReply struct {
	Measured              bool   `json:"measured"`
	ReadToPublishMicros   uint32 `json:"readToPublishMicros"`
	PublishToResumeMicros uint32 `json:"publishToResumeMicros"`
}

func CaptureReplyFromClocks(read, publish, resume uint64) CaptureReply {
	if read == 0 || publish < read || resume < publish || resume-read > uint64(Window.Microseconds()) {
		return CaptureReply{}
	}
	return CaptureReply{Measured: true, ReadToPublishMicros: uint32(publish - read), PublishToResumeMicros: uint32(resume - publish)}
}

func (p CaptureReply) valid(total uint32) bool {
	if !p.Measured {
		return p == (CaptureReply{})
	}
	return uint64(p.ReadToPublishMicros)+uint64(p.PublishToResumeMicros) == uint64(total)
}
