package inputworker

import (
	"encoding/binary"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// Installed native parent/child images are hash-matched as one bundle.
// The outer journal timestamps coexist with the normalized inner child spans.
// One fixed suffix, no additional read/write/exchange.
const nativeIPCSize = 68

type nativeIPCWire struct {
	stamps      [3]uint64 // child read complete, handler entry, immediately before Write
	profile     *handoff.NativeIPCProfile
	outer       [3]uint64 // journal read complete, reply ready, write submission
	nativeWrite bool      // Outer submit was stamped after native pipe admission.
}

func (p *nativeIPCWire) encode(data []byte) {
	if p.outer != ([3]uint64{}) {
		data[0] = 4
		if p.nativeWrite {
			data[1] = 4
		}
		if p.profile != nil {
			data[1] |= 1
			if p.profile.ParentMeasured {
				data[1] |= 2
			}
			if p.profile.RequestWriteMeasured {
				data[1] |= 8
				binary.BigEndian.PutUint32(data[4:8], p.profile.RequestWriteMicros)
			}
			encodeNormalizedIPC(data, p.profile)
		}
		for i, n := range p.outer {
			binary.BigEndian.PutUint64(data[44+i*8:], n)
		}
		return
	}
	if p.profile != nil {
		data[0] = 2
		if p.profile.ParentMeasured {
			data[0] = 3
		}
		if p.profile.RequestWriteMeasured {
			data[0] = 5
			if p.profile.ParentMeasured {
				data[1] = 1
			}
			binary.BigEndian.PutUint32(data[4:8], p.profile.RequestWriteMicros)
		}
		encodeNormalizedIPC(data, p.profile)
		return
	}
	data[0] = 1
	for i, n := range p.stamps {
		binary.BigEndian.PutUint64(data[8+i*8:], n)
	}
}

func encodeNormalizedIPC(data []byte, p *handoff.NativeIPCProfile) {
	for i, n := range p.Micros {
		binary.BigEndian.PutUint32(data[8+i*4:], n)
	}
	if p.ParentMeasured {
		for i, n := range p.ParentMicros {
			binary.BigEndian.PutUint32(data[32+i*4:], n)
		}
	}
}

func decodeNativeIPC(data []byte) *nativeIPCWire {
	if len(data) != nativeIPCSize {
		return nil
	}
	if data[0] == 4 {
		if !zeroBytes(data[2:4]) || data[1] > 15 || data[1]&8 == 0 && !zeroBytes(data[4:8]) || data[1]&8 != 0 && data[1]&1 == 0 || data[1]&3 == 2 || data[1]&2 == 0 && !zeroBytes(data[32:44]) || data[1]&1 == 0 && !zeroBytes(data[8:44]) {
			return nil
		}
	} else if data[0] == 5 {
		if data[1] > 1 || !zeroBytes(data[2:4]) || !zeroBytes(data[44:]) || data[1] == 0 && !zeroBytes(data[32:44]) {
			return nil
		}
	} else if !zeroBytes(data[1:8]) || !zeroBytes(data[44:]) || data[0] != 3 && !zeroBytes(data[32:44]) {
		return nil
	}
	p := &nativeIPCWire{}
	if data[0] == 4 {
		p.nativeWrite = data[1]&4 != 0
		for i := range p.outer {
			p.outer[i] = binary.BigEndian.Uint64(data[44+i*8:])
		}
		if data[1]&1 == 0 {
			return p
		}
	}
	switch data[0] {
	case 1:
		for i := range p.stamps {
			p.stamps[i] = binary.BigEndian.Uint64(data[8+i*8:])
		}
	case 2, 3, 4, 5:
		profile := &handoff.NativeIPCProfile{}
		for i := range profile.Micros {
			profile.Micros[i] = binary.BigEndian.Uint32(data[8+i*4:])
		}
		if data[0] == 3 || data[0] == 4 && data[1]&2 != 0 || data[0] == 5 && data[1] == 1 {
			profile.ParentMeasured = true
			for i := range profile.ParentMicros {
				profile.ParentMicros[i] = binary.BigEndian.Uint32(data[32+i*4:])
			}
		}
		if data[0] == 5 || data[0] == 4 && data[1]&8 != 0 {
			profile.RequestWriteMeasured = true
			profile.RequestWriteMicros = binary.BigEndian.Uint32(data[4:8])
		}
		if !profile.Valid() {
			return nil
		}
		p.profile = profile
	default:
		return nil
	}
	return p
}

func recordNativeIPCWrite(p *handoff.NativeIPCProfile, submitted, written, complete uint64) {
	if p == nil || submitted == 0 || written < submitted || complete < written || complete-submitted > uint64(handoff.Window.Microseconds()) {
		return
	}
	copy := *p
	copy.RequestWriteMeasured, copy.RequestWriteMicros = true, uint32(written-submitted)
	if copy.Valid() {
		*p = copy
	}
}

// All stamps belong to this synchronous exchange. Invalid/over-budget parent
// metadata is dropped without replacing valid IPC timing or changing input.
func recordNativeIPCParent(p *handoff.NativeIPCProfile, entered, locked, started, decoded, finished uint64) {
	if p == nil || entered == 0 || locked < entered || started < locked || decoded < started || finished < decoded {
		return
	}
	if finished-entered > uint64(handoff.Window.Microseconds()) {
		return
	}
	parent := [3]uint32{uint32(locked - entered), uint32(started - locked), uint32(finished - decoded)}
	copy := *p
	copy.ParentMicros, copy.ParentMeasured = parent, true
	if copy.Valid() {
		*p = copy
	}
}

func normalizeNativeIPC(started, admitted, submitted uint64, wire *nativeIPCWire, first, complete uint64) *handoff.NativeIPCProfile {
	if wire == nil || wire.profile != nil {
		return nil
	}
	points := [...]uint64{started, admitted, submitted, wire.stamps[0], wire.stamps[1], wire.stamps[2], first, complete}
	for i, n := range points {
		if n == 0 || i > 0 && n < points[i-1] {
			return nil
		}
	}
	if complete-started > uint64(handoff.Window.Microseconds()) {
		return nil
	}
	// Request encoding/submission preparation belongs to executor admission.
	p := &handoff.NativeIPCProfile{Micros: [6]uint32{uint32(submitted - started)}}
	for i := 1; i < len(p.Micros); i++ {
		p.Micros[i] = uint32(points[i+2] - points[i+1])
	}
	return p
}

type nativeIPCReader struct {
	io.Reader
	first uint64
}

func (r *nativeIPCReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	if n > 0 && r.first == 0 {
		r.first = workerIPCTick()
	}
	return n, err
}
