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
}

func (p *sourceHeartbeatProgress) measure(stage byte) func() {
	p.stage = stage
	started := time.Now()
	return func() { p.record(stage, time.Since(started)) }
}

func (p *sourceHeartbeatProgress) record(stage byte, elapsed time.Duration) {
	bucket := timingBucket(elapsed)
	if bucket > p.slowBucket {
		p.slowStage, p.slowBucket = stage, bucket
	}
}

func (p *sourceHeartbeatProgress) snapshot(exit *logindiag.SourceExit) {
	exit.HeartbeatStage = logindiag.SourceHeartbeatStages[p.stage]
	exit.HeartbeatSlowStage = logindiag.SourceHeartbeatStages[p.slowStage]
	exit.HeartbeatSlowBucket = p.slowBucket
}
