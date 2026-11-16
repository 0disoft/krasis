package inputworker

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// KRD2-KRD9 are fixed, one-shot local readiness envelopes, never an input ACK or
// peer frame. Process/image authentication and desktop readiness still precede
// it. Legacy KRD1 has no diagnostic payload and remains readable.
const startupReadySize = 8 + int(handoff.StartupStageCount)*4
const startupReadyLoadSize = startupReadySize + 8
const startupReadyPreparedSize = startupReadyLoadSize + 8
const startupReadyDecisionSize = startupReadyPreparedSize + 8
const startupReadyWaitSize = startupReadyDecisionSize + 8
const startupReadyServiceSize = startupReadyWaitSize + 8 + int(handoff.ServiceReadinessLegacyStageCount)*4
const startupReadyNativeSize = startupReadyServiceSize + 48
const startupReadyImageHashSize = startupReadyNativeSize + 36
const startupReadyAdmissionSize = startupReadyImageHashSize + 16

func writeStartupReady(writer io.Writer, profile handoff.StartupProfile) error {
	if !profile.Valid() || profile.Measured == 0 {
		return writeFrame(writer, readyFrame[:])
	}
	var data [startupReadyAdmissionSize]byte
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
		data[startupReadyWaitSize] = profile.ServiceReadiness.Measured & ((1 << handoff.ServiceReadinessLegacyStageCount) - 1)
		for i, n := range profile.ServiceReadiness.Micros[:handoff.ServiceReadinessLegacyStageCount] {
			binary.BigEndian.PutUint32(data[startupReadyWaitSize+8+i*4:], n)
		}
	}
	data[5] = profile.Measured
	if profile.Native[0].Measured != 0 || profile.Native[1].Measured != 0 {
		copy(data[:5], []byte{'K', 'R', 'D', '8', 8})
		size = startupReadyNativeSize
		for i, native := range profile.Native {
			offset := startupReadyServiceSize + i*24
			data[offset] = native.Measured
			for j, n := range native.Micros {
				binary.BigEndian.PutUint32(data[offset+4+j*4:], n)
			}
		}
	}
	for i, n := range profile.Micros {
		binary.BigEndian.PutUint32(data[8+i*4:12+i*4], n)
	}
	if profile.ImageHash.Backend != 0 || profile.Native[0].ImageHash.Backend != 0 || profile.Native[1].ImageHash.Backend != 0 {
		copy(data[:5], []byte{'K', 'R', 'D', '9', 9})
		size = startupReadyImageHashSize
		for i, p := range [3]handoff.ImageHashProfile{profile.ImageHash, profile.Native[0].ImageHash, profile.Native[1].ImageHash} {
			offset := startupReadyNativeSize + i*12
			data[offset] = p.Backend
			for j, n := range p.Micros {
				binary.BigEndian.PutUint32(data[offset+4+j*4:], n)
			}
		}
	}
	if profile.ServiceReadiness.Measured>>handoff.ServiceReadinessLegacyStageCount != 0 {
		copy(data[:5], []byte{'K', 'R', 'D', 'A', 10})
		size = startupReadyAdmissionSize
		data[startupReadyImageHashSize] = profile.ServiceReadiness.Measured >> handoff.ServiceReadinessLegacyStageCount
		for i, n := range profile.ServiceReadiness.Micros[handoff.ServiceReadinessLegacyStageCount:] {
			binary.BigEndian.PutUint32(data[startupReadyImageHashSize+8+i*4:], n)
		}
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
	admission := string(header[:4]) == "KRDA" && header[4] == 10
	imageHash := admission || string(header[:4]) == "KRD9" && header[4] == 9
	native := imageHash || string(header[:4]) == "KRD8" && header[4] == 8
	service := native || string(header[:4]) == "KRD7" && header[4] == 7
	wait := service || (string(header[:4]) == "KRD6" && header[4] == 6)
	decision := wait || (string(header[:4]) == "KRD5" && header[4] == 5)
	prepared := decision || (string(header[:4]) == "KRD4" && header[4] == 4)
	load := prepared || (string(header[:4]) == "KRD3" && header[4] == 3)
	if (!load && (string(header[:4]) != "KRD2" || header[4] != 2)) || !zeroBytes(header[6:]) {
		return nil, ErrProtocol
	}
	// The version fixes the entire bounded receipt size. Read that body once;
	// per-field reads otherwise submit nine separate overlapped pipe reads for
	// KRD9. Never buffer beyond this receipt into the following input/report.
	size := startupReadySize
	switch {
	case admission:
		size = startupReadyAdmissionSize
	case imageHash:
		size = startupReadyImageHashSize
	case native:
		size = startupReadyNativeSize
	case service:
		size = startupReadyServiceSize
	case wait:
		size = startupReadyWaitSize
	case decision:
		size = startupReadyDecisionSize
	case prepared:
		size = startupReadyPreparedSize
	case load:
		size = startupReadyLoadSize
	}
	var body [startupReadyAdmissionSize - 8]byte
	if n, err := io.ReadFull(reader, body[:size-8]); err != nil {
		// Preserve EOF at former field boundaries, versus UnexpectedEOF in
		// a partial field. Native deadline/cancellation errors pass unchanged.
		if err == io.ErrUnexpectedEOF {
			switch n + len(header) {
			case startupReadySize, startupReadyLoadSize, startupReadyPreparedSize,
				startupReadyDecisionSize, startupReadyWaitSize, startupReadyServiceSize,
				startupReadyNativeSize, startupReadyImageHashSize:
				err = io.EOF
			}
		}
		return nil, errors.Join(ErrProtocol, err)
	}
	payload := body[:startupReadySize-8]
	profile := &handoff.StartupProfile{Schema: 1, Measured: header[5]}
	for i := range profile.Micros {
		profile.Micros[i] = binary.BigEndian.Uint32(payload[i*4 : i*4+4])
	}
	if load {
		detail := body[startupReadySize-8 : startupReadyLoadSize-8]
		if detail[4] > 1 || !zeroBytes(detail[5:]) {
			return nil, ErrProtocol
		}
		profile.LoadMicros = binary.BigEndian.Uint32(detail[:4])
		profile.LoadMeasured = detail[4] == 1
	}
	if prepared {
		detail := body[startupReadyLoadSize-8 : startupReadyPreparedSize-8]
		if detail[4] > 1 || (!decision && detail[4] != 1) || !zeroBytes(detail[5:]) {
			return nil, ErrProtocol
		}
		profile.Prepared, profile.PreparedAgeMicros = detail[4] == 1, binary.BigEndian.Uint32(detail[:4])
	}
	if decision {
		detail := body[startupReadyPreparedSize-8 : startupReadyDecisionSize-8]
		if !zeroBytes(detail[1:]) {
			return nil, ErrProtocol
		}
		profile.PreparationDecision = handoff.PreparationDecision(detail[0])
	}
	if wait {
		detail := body[startupReadyDecisionSize-8 : startupReadyWaitSize-8]
		if detail[4] > 1 || !zeroBytes(detail[5:]) {
			return nil, ErrProtocol
		}
		profile.PreparationWaitMicros = binary.BigEndian.Uint32(detail[:4])
		profile.PreparationWaitMeasured = detail[4] == 1
	}
	invalidLegacyServiceMask := false
	if service {
		detail := body[startupReadyWaitSize-8 : startupReadyServiceSize-8]
		if !zeroBytes(detail[1:8]) {
			return nil, ErrProtocol
		}
		profile.ServiceReadiness.Measured = detail[0]
		invalidLegacyServiceMask = detail[0]>>handoff.ServiceReadinessLegacyStageCount != 0
		for i := range profile.ServiceReadiness.Micros[:handoff.ServiceReadinessLegacyStageCount] {
			profile.ServiceReadiness.Micros[i] = binary.BigEndian.Uint32(detail[8+i*4:])
		}
	}
	if native {
		detail := body[startupReadyServiceSize-8 : startupReadyNativeSize-8]
		for i := range profile.Native {
			offset := i * 24
			if !zeroBytes(detail[offset+1 : offset+4]) {
				return nil, ErrProtocol
			}
			profile.Native[i].Measured = detail[offset]
			for j := range profile.Native[i].Micros {
				profile.Native[i].Micros[j] = binary.BigEndian.Uint32(detail[offset+4+j*4:])
			}
		}
	}
	if imageHash {
		detail := body[startupReadyNativeSize-8 : startupReadyImageHashSize-8]
		for i, p := range [3]*handoff.ImageHashProfile{&profile.ImageHash, &profile.Native[0].ImageHash, &profile.Native[1].ImageHash} {
			offset := i * 12
			if !zeroBytes(detail[offset+1 : offset+4]) {
				return nil, ErrProtocol
			}
			p.Backend = detail[offset]
			for j := range p.Micros {
				p.Micros[j] = binary.BigEndian.Uint32(detail[offset+4+j*4:])
			}
		}
	}
	if admission {
		detail := body[startupReadyImageHashSize-8 : startupReadyAdmissionSize-8]
		if !zeroBytes(detail[1:8]) || detail[0] == 0 || detail[0]>>2 != 0 {
			return nil, ErrProtocol
		}
		profile.ServiceReadiness.Measured |= detail[0] << handoff.ServiceReadinessLegacyStageCount
		for i := range profile.ServiceReadiness.Micros[handoff.ServiceReadinessLegacyStageCount:] {
			profile.ServiceReadiness.Micros[int(handoff.ServiceReadinessLegacyStageCount)+i] = binary.BigEndian.Uint32(detail[8+i*4:])
		}
	}
	// Numeric diagnostics are not readiness proof. Consume the fixed payload,
	// but drop invalid/missing measurements without changing a valid receipt.
	if invalidLegacyServiceMask || !profile.Valid() || profile.Measured == 0 || decision && !service && profile.PreparationDecision == handoff.PreparationUnobserved || service && !native && profile.ServiceReadiness.Measured == 0 {
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
	service.Native = worker.Native
	service.ImageHash = worker.ImageHash
}
