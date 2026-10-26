package inputworker

import "github.com/0disoft/krasis/internal/handoff"

// Seventeen three-bit stages use 51 previously reserved bits of the fixed
// activation ACK. No additional pipe frame or input/authority field is sent.
func packSourceCapture(p handoff.CaptureNative) uint64 {
	var bits uint64
	for i, bucket := range p {
		bits |= uint64(bucket) << (3 * i)
	}
	return bits
}

func unpackSourceCapture(bits uint64) (handoff.CaptureNative, bool) {
	var p handoff.CaptureNative
	if bits>>(3*len(p)) != 0 {
		return p, false
	}
	for i := range p {
		p[i] = byte(bits >> (3 * i) & 7)
	}
	return p, p.Valid()
}
