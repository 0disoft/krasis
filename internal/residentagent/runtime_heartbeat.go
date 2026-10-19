package residentagent

import (
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/productlog"
)

// Each leg has one producer: the serial pulse sender or serial frame reader.
// Sequences fence mixed samples. A snapshot retries at most three times, then
// omits that leg rather than waiting or exporting a guessed sequence.
type heartbeatProgressLeg struct {
	sequence          atomic.Uint64
	times             [4]atomic.Int64
	measured, outcome atomic.Uint32
	lastComplete      atomic.Int64
}

func (p *heartbeatProgressLeg) observe(stage peer.HeartbeatStage, stamp int64) {
	if stage > peer.HeartbeatFailed {
		return
	}
	p.sequence.Add(1)
	if stage == peer.HeartbeatStart {
		for i := range p.times {
			p.times[i].Store(0)
		}
		p.measured.Store(0)
		p.outcome.Store(0)
	}
	index := uint8(stage)
	if stage >= peer.HeartbeatComplete {
		index = 3
		// Preserve the first terminal boundary (including pre-cleanup error).
		if p.outcome.Load() != 0 {
			p.sequence.Add(1)
			return
		}
		p.outcome.Store(uint32(stage-peer.HeartbeatComplete) + 1)
		if stage == peer.HeartbeatComplete {
			p.lastComplete.Store(stamp)
		}
	}
	p.times[index].Store(stamp)
	p.measured.Store(p.measured.Load() | 1<<index)
	p.sequence.Add(1)
}

func (p *heartbeatProgressLeg) snapshot() productlog.HeartbeatLeg {
	for attempt := 0; attempt < 3; attempt++ {
		version := p.sequence.Load()
		if version&1 != 0 {
			continue
		}
		value := productlog.HeartbeatLeg{Measured: uint8(p.measured.Load()), Outcome: uint8(p.outcome.Load()), LastCompleteMicros: p.lastComplete.Load()}
		for i := range value.Micros {
			value.Micros[i] = p.times[i].Load()
		}
		if p.sequence.Load() == version {
			return value
		}
	}
	return productlog.HeartbeatLeg{}
}

func (p *heartbeatProgressLeg) reset() {
	p.sequence.Add(1)
	for i := range p.times {
		p.times[i].Store(0)
	}
	p.measured.Store(0)
	p.outcome.Store(0)
	p.lastComplete.Store(0)
	p.sequence.Add(1)
}

func (d *runtimeDiagnostics) ObserveHeartbeatSend(stage peer.HeartbeatStage, at time.Time) {
	d.heartbeatSend.observe(stage, d.stamp(at))
}
func (d *runtimeDiagnostics) ObserveHeartbeatReceive(stage peer.HeartbeatStage, at time.Time) {
	d.heartbeatReceive.observe(stage, d.stamp(at))
}

func (d *runtimeDiagnostics) heartbeatSnapshot() productlog.HeartbeatProgress {
	return productlog.HeartbeatProgress{Schema: 1, Send: d.heartbeatSend.snapshot(), Receive: d.heartbeatReceive.snapshot(), LastSentMicros: d.send.Load(), LastReceivedMicros: d.receive.Load()}
}
