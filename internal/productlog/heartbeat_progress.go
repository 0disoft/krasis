package productlog

// HeartbeatLeg retains one local pulse, never a peer/session identity.
// Measured declares timestamps in Micros. Outcome: 0 pending, 1 successful
// local write/dispatch, 2 inactive sender, 3 failed. Receive success can drain
// a validated retired heartbeat; it does not certify current lease renewal.
type HeartbeatLeg struct {
	Micros             [4]int64 `json:"micros"`
	Measured           uint8    `json:"measured"`
	Outcome            uint8    `json:"outcome"`
	LastCompleteMicros int64    `json:"lastCompleteMicros"`
}

type HeartbeatProgress struct {
	Schema             uint8        `json:"schema"`
	AtMicros           int64        `json:"atMicros"`
	Send               HeartbeatLeg `json:"send"`
	Receive            HeartbeatLeg `json:"receive"`
	LastSentMicros     int64        `json:"lastSentMicros"`
	LastReceivedMicros int64        `json:"lastReceivedMicros"`
}

func (p HeartbeatLeg) valid(at int64, receive bool) bool {
	if p.Measured == 0 {
		return p == (HeartbeatLeg{})
	}
	if p.LastCompleteMicros < 0 || p.LastCompleteMicros > at || p.Outcome == 1 && p.LastCompleteMicros != p.Micros[3] || p.Outcome != 1 && p.LastCompleteMicros > p.Micros[0] {
		return false
	}
	if p.Measured != 1 && p.Measured != 3 && p.Measured != 7 && p.Measured != 9 && p.Measured != 11 && p.Measured != 15 {
		return false
	}
	terminal := p.Measured&8 != 0
	if terminal != (p.Outcome != 0) || p.Outcome > 3 || p.Outcome == 1 && p.Measured != 15 || p.Outcome == 2 && (receive || p.Measured != 11) {
		return false
	}
	previous := int64(0)
	for i, n := range p.Micros {
		if p.Measured&(1<<i) == 0 {
			if n != 0 {
				return false
			}
			continue
		}
		if n < 1 || n > at || n < previous {
			return false
		}
		previous = n
	}
	return true
}

func (p HeartbeatProgress) valid() bool {
	return p.Schema == 1 && p.AtMicros > 0 && p.LastSentMicros >= 0 && p.LastSentMicros <= p.AtMicros &&
		p.LastReceivedMicros >= 0 && p.LastReceivedMicros <= p.AtMicros && p.Send.valid(p.AtMicros, false) && p.Receive.valid(p.AtMicros, true)
}
