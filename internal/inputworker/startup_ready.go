package inputworker

import (
	"encoding/binary"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// KRD2 is a fixed, one-shot local readiness envelope, never an input ACK or
// peer frame. Process/image authentication and desktop readiness still precede
// it. Legacy KRD1 has no diagnostic payload and remains readable.
const startupReadySize = 8 + int(handoff.StartupStageCount)*4

func writeStartupReady(writer io.Writer, profile handoff.StartupProfile) error {
	if !profile.Valid() || profile.Measured == 0 {
		return writeFrame(writer, readyFrame[:])
	}
	var data [startupReadySize]byte
	copy(data[:5], []byte{'K', 'R', 'D', '2', 2})
	data[5] = profile.Measured
	for i, n := range profile.Micros {
		binary.BigEndian.PutUint32(data[8+i*4:12+i*4], n)
	}
	return writeFrame(writer, data[:])
}

func readStartupReady(reader io.Reader) (*handoff.StartupProfile, error) {
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, ErrProtocol
	}
	if header == readyFrame {
		return nil, nil
	}
	if string(header[:4]) != "KRD2" || header[4] != 2 || !zeroBytes(header[6:]) {
		return nil, ErrProtocol
	}
	var payload [startupReadySize - 8]byte
	if _, err := io.ReadFull(reader, payload[:]); err != nil {
		return nil, ErrProtocol
	}
	profile := &handoff.StartupProfile{Schema: 1, Measured: header[5]}
	for i := range profile.Micros {
		profile.Micros[i] = binary.BigEndian.Uint32(payload[i*4 : i*4+4])
	}
	// Numeric diagnostics are not readiness proof. Consume the fixed payload,
	// but drop invalid/missing measurements without changing a valid receipt.
	if !profile.Valid() || profile.Measured == 0 {
		return nil, nil
	}
	return profile, nil
}

func mergeWorkerStartup(service *handoff.StartupProfile, worker *handoff.StartupProfile) {
	if service == nil || worker == nil || !worker.Valid() {
		return
	}
	// Child measurements cannot replace service-local measurements.
	for stage := handoff.WorkerInstalledValidation; stage < handoff.StartupStageCount; stage++ {
		if worker.Measured&(1<<stage) != 0 {
			service.Schema = 1
			service.Measured |= 1 << stage
			service.Micros[stage] = worker.Micros[stage]
		}
	}
}
