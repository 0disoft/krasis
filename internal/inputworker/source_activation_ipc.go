package inputworker

import (
	"encoding/binary"
	"github.com/0disoft/krasis/internal/handoff"
)

// A versioned optional 48-byte suffix on this private activation ACK only.
// It is written with its header in ONE pipe operation. Other packets stay 64B.
// Old replies remain readable; an old reader fails closed on the reserved flag.
const sourceActivationIPCFlag byte = 0x80
const sourceActivationIPCSize = 6 * 8
const sourceActivationAuthorityFlag byte = 0x81
const sourceActivationAuthoritySize = 2 * 4

type sourceActivationClocks [6]uint64

// Only activation packets allocate this; input queues retain one nil pointer
// rather than copying eight diagnostic timestamps for every pointer event.
type sourceActivationTrace struct {
	clocks        sourceActivationClocks
	received      uint64
	readAuthority handoff.CaptureAuthorityProfile
}

func (c sourceActivationClocks) usable() bool {
	if c[0] == 0 {
		return false
	}
	for i := 1; i < len(c); i++ {
		if c[i] < c[i-1] {
			return false
		}
	}
	return true
}

func appendSourceActivationIPC(header [sourcePacketSize]byte, c sourceActivationClocks, gate, write uint64, details ...handoff.CaptureAuthorityProfile) []byte {
	c[4], c[5] = gate, write
	if !c.usable() {
		return header[:]
	}
	var wire [sourcePacketSize + sourceActivationIPCSize + sourceActivationAuthoritySize]byte
	copy(wire[:], header[:])
	wire[56] = sourceActivationIPCFlag
	for i, tick := range c {
		binary.BigEndian.PutUint64(wire[sourcePacketSize+i*8:], tick)
	}
	if len(details) > 0 && details[0].Measured && details[0].Valid() {
		wire[56] = sourceActivationAuthorityFlag
		tail := wire[sourcePacketSize+sourceActivationIPCSize:]
		binary.BigEndian.PutUint32(tail[:4], details[0].WaitMicros+1)
		binary.BigEndian.PutUint32(tail[4:], details[0].NativeMicros+1)
		return wire[:]
	}
	return wire[:sourcePacketSize+sourceActivationIPCSize]
}
