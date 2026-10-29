package handoff

// Diagnostic durations for the full-image hash, not authentication proofs.
// Backend 1 is Go SHA256, 2 is Windows CNG SHA256, 0 is unmeasured.
type ImageHashProfile struct {
	Micros  [2]uint32 `json:"micros"` // file reads, digest creation/writes/finish/close
	Backend uint8     `json:"backend"`
}

func (p ImageHashProfile) Valid() bool {
	if p.Backend > 2 {
		return false
	}
	for _, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || p.Backend == 0 && n != 0 {
			return false
		}
	}
	return true
}
