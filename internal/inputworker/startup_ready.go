package inputworker

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// KRD2-KRD7 are fixed, one-shot local readiness envelopes, never an input ACK or
// peer frame. Process/image authentication and desktop readiness still precede
// it. Legacy KRD1 has no diagnostic payload and remains readable.
const startupReadySize = 8 + int(handoff.StartupStageCount)*4
const startupReadyLoadSize = startupReadySize + 8
const startupReadyPreparedSize = startupReadyLoadSize + 8
const startupReadyDecisionSize = startupReadyPreparedSize + 8
const startupReadyWaitSize = startupReadyDecisionSize + 8
const startupReadyServiceSize = startupReadyWaitSize + 8 + int(handoff.ServiceReadinessStageCount)*4

func writeStartupReady(writer io.Writer, profile handoff.StartupProfile) error {
	if !profile.Valid() || profile.Measured == 0 {
		return writeFrame(writer, readyFrame[:])
	}
	var data [startupReadyServiceSize]byte
	copy(data[:5], []byte{'K', 'R', 'D', '2', 2})
	size := startupReadySize
	if profile.LoadMeasured {
		copy(data[:5], []byte{'K', 'R', 'D', '3', 3})
		size = startupReadyLoadSize
		binary.BigEndian.PutUint32(data[startupReadySize:startupReadySize+4], profile.LoadMicros)
		data[startupReadySize+4] = 1
	}
	if profile.Prepared {
		copy(data[:5], []byte{'K', 'R', 'D', '4', 4})
		size = startupReadyPreparedSize
		binary.BigEndian.PutUint32(data[startupReadyLoadSize:startupReadyLoadSize+4], profile.PreparedAgeMicros)
		data[startupReadyLoadSize+4] = 1
	}
	if profile.PreparationDecision != handoff.PreparationUnobserved {
		copy(data[:5], []byte{'K', 'R', 'D', '5', 5})
		size = startupReadyDecisionSize
		data[startupReadyPreparedSize] = byte(profile.PreparationDecision)
	}
	if profile.PreparationWaitMeasured {
		copy(data[:5], []byte{'K', 'R', 'D', '6', 6})
		size = startupReadyWaitSize
		binary.BigEndian.PutUint32(data[startupReadyDecisionSize:startupReadyDecisionSize+4], profile.PreparationWaitMicros)
		data[startupReadyDecisionSize+4] = 1
	}
	if profile.ServiceReadiness.Measured != 0 {
		copy(data[:5], []byte{'K', 'R', 'D', '7', 7})
		size = startupReadyServiceSize
		data[startupReadyWaitSize] = profile.ServiceReadiness.Measured
		for i, n := range profile.ServiceReadiness.Micros {
			binary.BigEndian.PutUint32(data[startupReadyWaitSize+8+i*4:], n)
		}
	}
	data[5] = profile.Measured
	for i, n := range profile.Micros {
		binary.BigEndian.PutUint32(data[8+i*4:12+i*4], n)
	}
	return writeFrame(writer, data[:size])
}

func readStartupReady(reader io.Reader) (*handoff.StartupProfile, error) {
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, errors.Join(ErrProtocol, err)
	}
	if header == readyFrame {
		return nil, nil
	}
	service := string(header[:4]) == "KRD7" && header[4] == 7
	wait := service || (string(header[:4]) == "KRD6" && header[4] == 6)
	decision := wait || (string(header[:4]) == "KRD5" && header[4] == 5)
	prepared := decision || (string(header[:4]) == "KRD4" && header[4] == 4)
	load := prepared || (string(header[:4]) == "KRD3" && header[4] == 3)
	if (!load && (string(header[:4]) != "KRD2" || header[4] != 2)) || !zeroBytes(header[6:]) {
		return nil, ErrProtocol
	}
	var payload [startupReadySize - 8]byte
	if _, err := io.ReadFull(reader, payload[:]); err != nil {
		return nil, errors.Join(ErrProtocol, err)
	}
	profile := &handoff.StartupProfile{Schema: 1, Measured: header[5]}
	for i := range profile.Micros {
		profile.Micros[i] = binary.BigEndian.Uint32(payload[i*4 : i*4+4])
	}
	if load {
		var detail [8]byte
		if _, err := io.ReadFull(reader, detail[:]); err != nil {
			return nil, errors.Join(ErrProtocol, err)
		}
		if detail[4] > 1 || !zeroBytes(detail[5:]) {
			return nil, ErrProtocol
		}
		profile.LoadMicros = binary.BigEndian.Uint32(detail[:4])
		profile.LoadMeasured = detail[4] == 1
	}
	if prepared {
		var detail [8]byte
		if _, err := io.ReadFull(reader, detail[:]); err != nil {
			return nil, errors.Join(ErrProtocol, err)
		}
		if detail[4] > 1 || (!decision && detail[4] != 1) || !zeroBytes(detail[5:]) {
			return nil, ErrProtocol
		}
		profile.Prepared, profile.PreparedAgeMicros = detail[4] == 1, binary.BigEndian.Uint32(detail[:4])
	}
	if decision {
		var detail [8]byte
		if _, err := io.ReadFull(reader, detail[:]); err != nil {
			return nil, errors.Join(ErrProtocol, err)
		}
		if !zeroBytes(detail[1:]) {
			return nil, ErrProtocol
		}
		profile.PreparationDecision = handoff.PreparationDecision(detail[0])
	}
	if wait {
		var detail [8]byte
		if _, err := io.ReadFull(reader, detail[:]); err != nil {
			return nil, errors.Join(ErrProtocol, err)
		}
		if detail[4] > 1 || !zeroBytes(detail[5:]) {
			return nil, ErrProtocol
		}
		profile.PreparationWaitMicros = binary.BigEndian.Uint32(detail[:4])
		profile.PreparationWaitMeasured = detail[4] == 1
	}
	if service {
		var detail [startupReadyServiceSize - startupReadyWaitSize]byte
		if _, err := io.ReadFull(reader, detail[:]); err != nil {
			return nil, errors.Join(ErrProtocol, err)
		}
		if !zeroBytes(detail[1:8]) {
			return nil, ErrProtocol
		}
		profile.ServiceReadiness.Measured = detail[0]
		for i := range profile.ServiceReadiness.Micros {
			profile.ServiceReadiness.Micros[i] = binary.BigEndian.Uint32(detail[8+i*4:])
		}
	}
	// Numeric diagnostics are not readiness proof. Consume the fixed payload,
	// but drop invalid/missing measurements without changing a valid receipt.
	if !profile.Valid() || profile.Measured == 0 || decision && !service && profile.PreparationDecision == handoff.PreparationUnobserved || service && profile.ServiceReadiness.Measured == 0 {
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
	if worker.LoadMeasured {
		service.LoadMicros, service.LoadMeasured = worker.LoadMicros, true
	}
}
