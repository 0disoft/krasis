package inputworker

// Publication remains nonblocking and FIFO. The native reader is locked to an
// OS thread; after waking the activation caller, yield its Go processor before
// the next blocking Windows pipe read can retain it until sysmon retakes it.
// Ordinary input and maintenance replies never pay this scheduling handoff.
func publishSourceReply(replies chan<- sourcePacket, packet sourcePacket, yield func()) bool {
	select {
	case replies <- packet:
		if packet.op == sourceActivate {
			yield()
		}
		return true
	default:
		return false
	}
}
