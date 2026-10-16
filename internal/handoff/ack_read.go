package handoff

import (
	"context"
	"time"
)

const (
	ACKReadNoData uint8 = iota
	ACKReadData
	ACKReadHeader
	ACKReadComplete
	ACKReadMatched
)

// ACKReadProfile contains three disjoint client-local spans: start to the first
// positive Read return, then header decode, then suffix decode. It measures
// observation by the client, not kernel byte arrival or worker scheduling.
type ACKReadProfile struct {
	Micros    [3]uint32 `json:"micros"`
	Measured  uint8     `json:"measured"`
	Saturated uint8     `json:"saturated,omitempty"`
	Progress  uint8     `json:"progress"`
}

func (p ACKReadProfile) Valid() bool {
	if p.Progress > ACKReadMatched || p.Measured>>3 != 0 || p.Saturated & ^p.Measured != 0 {
		return false
	}
	expected := uint8(7)
	if p.Progress == ACKReadNoData {
		expected = 1
	} else if p.Progress == ACKReadData {
		expected = 3
	}
	if p.Measured != expected {
		return false
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || p.Measured&(1<<i) == 0 && n != 0 || p.Saturated&(1<<i) != 0 && n != uint32(Window.Microseconds()) {
			return false
		}
	}
	return true
}

func (p ACKReadProfile) validForResponse(v Vector) bool {
	if !p.Valid() || v.Measured&(1<<ClientResponse) == 0 {
		return false
	}
	// Floored disjoint microsecond spans must fit their unsaturated parent.
	// A saturated parent gives no bound for their sum, not a latency estimate.
	if v.Micros[ClientResponse] < Window.Microseconds() {
		return uint64(p.Micros[0])+uint64(p.Micros[1])+uint64(p.Micros[2]) <= uint64(max(0, v.Micros[ClientResponse]))
	}
	return true
}

func (p *ACKReadProfile) Record(stage int, elapsed time.Duration) {
	if p == nil || stage < 0 || stage >= len(p.Micros) || p.Measured&(1<<stage) != 0 {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
		p.Saturated |= 1 << stage
	}
	p.Micros[stage] = uint32(elapsed.Microseconds())
	p.Measured |= 1 << stage
}

// Reuse the already-selected worker probe; no new budget or wire flag. The
// read profile also survives a lost/foreign ACK, but worker details do not.
func RecordACKRead(ctx context.Context, p *ACKReadProfile) {
	if ctx == nil {
		return
	}
	v, _ := ctx.Value(sampleKey{}).(*sample)
	if v == nil || !v.workerApplyRequested || v.ackReadRecorded {
		return
	}
	v.ackReadRecorded = true
	if p != nil && p.validForResponse(v.vector) {
		copy := *p
		v.vector.ACKRead = &copy
	}
}
