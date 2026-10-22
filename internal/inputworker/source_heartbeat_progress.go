package inputworker

import (
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
)

const (
	heartbeatServicePresence byte = iota + 1
	heartbeatAgentPresence
	heartbeatReplyGate
	heartbeatReplyPresence
	heartbeatReplyWrite
	heartbeatReplied
)

// Owned only by the serial command reader, including its heartbeat writer.
// Snapshot after that reader joins. No clocks, input values or identities leave
// this process: only fixed stages and the lifetime's largest elapsed bucket.
type sourceHeartbeatProgress struct {
	stage, slowStage, slowBucket byte
	presence                     sourcePresenceTiming
	gateBucket                   byte
}

func (p *sourceHeartbeatProgress) measure(stage byte) func() {
	p.stage = stage
	started := time.Now()
	return func() { p.record(stage, time.Since(started)) }
}

func (p *sourceHeartbeatProgress) record(stage byte, elapsed time.Duration) {
	bucket := timingBucket(elapsed)
	if stage == heartbeatReplyGate {
		p.gateBucket = bucket
	}
	if bucket > p.slowBucket {
		p.slowStage, p.slowBucket = stage, bucket
	}
}

func (p *sourceHeartbeatProgress) replyTiming() (buckets [3]byte) {
	if p.presence.measured {
		buckets[0], buckets[1] = timingBucket(p.presence.wait), timingBucket(p.presence.native)
	}
	buckets[2] = p.gateBucket
	return
}

func (p *sourceHeartbeatProgress) snapshot(exit *logindiag.SourceExit) {
	exit.HeartbeatStage = logindiag.SourceHeartbeatStages[p.stage]
	exit.HeartbeatSlowStage = logindiag.SourceHeartbeatStages[p.slowStage]
	exit.HeartbeatSlowBucket = p.slowBucket
}
