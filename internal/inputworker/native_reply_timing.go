package inputworker

// The native child uses the existing ACK timing fields, with no new frame or
// input probe. Parent publication requires exactly one successful, matching
// native exchange during this operation. Composite/failed/idle work stays
// unobserved rather than borrowing an earlier command's stages.
type nativeReplyTiming struct {
	sequence uint64
	timing   workerTiming
}

func (client *Client) nativeTimingSnapshot() (uint64, nativeReplyTiming) {
	// Diagnostics must never wait behind input, heartbeat or native cleanup.
	if !client.mu.TryLock() {
		return 0, nativeReplyTiming{}
	}
	defer client.mu.Unlock()
	return client.sequence, client.nativeReply
}

func (client *Client) recordNativeTiming(before uint64, timing *workerTiming) {
	after, reply := client.nativeTimingSnapshot()
	if timing == nil || before == 0 || after != before+1 || reply.sequence != before {
		return
	}
	for i := 0; i < 2; i++ {
		if reply.timing.measured&(1<<i) != 0 {
			timing.buckets[i], timing.millis[i] = reply.timing.buckets[i], reply.timing.millis[i]
			timing.measured |= 1 << i
		}
	}
}
