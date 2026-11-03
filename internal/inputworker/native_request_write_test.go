package inputworker

import (
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestNativeRequestWriteProfileRoundTripAndReservedBytes(t *testing.T) {
	for _, parent := range []bool{false, true} {
		for _, outer := range []bool{false, true} {
			p := &handoff.NativeIPCProfile{Micros: [6]uint32{1, 11, 0, 33, 4, 1}, ParentMeasured: parent}
			// Write completion can follow child read; this is a nested span.
			recordNativeIPCWrite(p, 100, 120, 150)
			if !p.RequestWriteMeasured || p.RequestWriteMicros != 20 {
				t.Fatal("write/read overlap was discarded", p)
			}
			wire := &nativeIPCWire{profile: p}
			if outer {
				wire.outer = [3]uint64{1, 2, 3}
				wire.nativeWrite = true
			}
			var data [nativeIPCSize]byte
			wire.encode(data[:])
			decoded := decodeNativeIPC(data[:])
			if decoded == nil || decoded.profile == nil || *decoded.profile != *p || decoded.outer != wire.outer || decoded.nativeWrite != wire.nativeWrite {
				t.Fatal("write diagnostic changed envelope", decoded)
			}
			data[2] = 1
			if decodeNativeIPC(data[:]) != nil {
				t.Fatal("reserved bytes accepted")
			}
		}
	}
}

func TestNativeRequestWriteRejectsClockRegressionAndBadBounds(t *testing.T) {
	for _, clocks := range [][3]uint64{{0, 1, 2}, {100, 99, 150}, {100, 121, 120}, {100, 200, 201}} {
		p := handoff.NativeIPCProfile{Micros: [6]uint32{50}}
		recordNativeIPCWrite(&p, clocks[0], clocks[1], clocks[2])
		if p.RequestWriteMeasured {
			t.Fatal("invalid or out-of-envelope write accepted", clocks)
		}
	}
	p := handoff.NativeIPCProfile{Micros: [6]uint32{50}}
	recordNativeIPCWrite(&p, 100, 150, 150)
	if !p.RequestWriteMeasured || p.RequestWriteMicros != 50 {
		t.Fatal("exact upper bound rejected")
	}
	p.RequestWriteMeasured = false
	if p.Valid() {
		t.Fatal("write without presence bit accepted")
	}
}
