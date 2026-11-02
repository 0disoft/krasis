package inputworker

import (
	"encoding/binary"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// Installed native parent/child images are hash-matched as one bundle.
// Twelve fixed bytes add parent spans without another read/write/exchange.
const nativeIPCSize = 44

type nativeIPCWire struct {
	stamps  [3]uint64 // child read complete, handler entry, immediately before Write
	profile *handoff.NativeIPCProfile
}

func (p *nativeIPCWire) encode(data []byte) {
	if p.profile != nil {
		data[0] = 2
		for i, n := range p.profile.Micros {
			binary.BigEndian.PutUint32(data[8+i*4:], n)
		}
		if p.profile.ParentMeasured {
			data[0] = 3
			for i, n := range p.profile.ParentMicros {
				binary.BigEndian.PutUint32(data[32+i*4:], n)
			}
		}
		return
	}
	data[0] = 1
	for i, n := range p.stamps {
		binary.BigEndian.PutUint64(data[8+i*8:], n)
	}
}

func decodeNativeIPC(data []byte) *nativeIPCWire {
	if len(data) != nativeIPCSize || !zeroBytes(data[1:8]) || data[0] != 3 && !zeroBytes(data[32:]) {
		return nil
	}
	p := &nativeIPCWire{}
	switch data[0] {
	case 1:
		for i := range p.stamps {
			p.stamps[i] = binary.BigEndian.Uint64(data[8+i*8:])
		}
	case 2, 3:
		profile := &handoff.NativeIPCProfile{}
		for i := range profile.Micros {
			profile.Micros[i] = binary.BigEndian.Uint32(data[8+i*4:])
		}
		if data[0] == 3 {
			profile.ParentMeasured = true
			for i := range profile.ParentMicros {
				profile.ParentMicros[i] = binary.BigEndian.Uint32(data[32+i*4:])
			}
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
