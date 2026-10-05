package inputworker

import (
	"encoding/binary"
	"io"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

const workerApplyDetailSize = 12
const workerSubmissionDetailSize = 8

func encodeWorkerApplyDetail(data []byte, p handoff.WorkerApplyProfile) {
	binary.BigEndian.PutUint32(data[:4], p.Micros[0])
	binary.BigEndian.PutUint32(data[4:8], p.Micros[1])
	data[8], data[9] = p.Measured, p.Saturated
}

func encodeWorkerSubmission(data []byte, p handoff.WorkerApplyProfile) {
	binary.BigEndian.PutUint32(data[:4], p.SubmissionMicros)
	data[4] = p.SubmissionCalls
	if p.SubmissionSaturated {
		data[5] = 1
	}
}

// Read the diagnostic and private cursor suffix together. A profiled cursor
// response retains two reads and one write, with no extra request or roundtrip.
func readResponseSuffix(reader io.Reader, version byte, value *response) error {
	var data [workerApplyDetailSize + workerSubmissionDetailSize + 18]byte
	size := 0
	if version >= 4 {
		size = workerApplyDetailSize
	}
	if version == 5 {
		size += workerSubmissionDetailSize
	}
	cursor := (value.op == opCursorRead || value.op == opMoveRead || value.op == opCursorPlace || value.op == opCursorPlaceProbe) && value.status == 0
	if cursor {
		size += 8
		if value.op == opCursorPlace || value.op == opCursorPlaceProbe {
			size += 8
		}
		if value.op == opCursorPlaceProbe {
			size += 2
		}
	}
	if size == 0 {
		return nil
	}
	if _, err := io.ReadFull(reader, data[:size]); err != nil {
		return ErrProtocol
	}
	offset := 0
	if version >= 4 {
		if data[10] != 0 || data[11] != 0 || value.op == opHeartbeat || value.op == opClose {
			return ErrProtocol
		}
		p := handoff.WorkerApplyProfile{Micros: [2]uint32{binary.BigEndian.Uint32(data[:4]), binary.BigEndian.Uint32(data[4:8])}, Measured: data[8], Saturated: data[9]}
		offset = workerApplyDetailSize
		if version == 5 {
			d := data[offset : offset+workerSubmissionDetailSize]
			if d[5] > 1 || d[6] != 0 || d[7] != 0 {
				return ErrProtocol
			}
			p.SubmissionMicros, p.SubmissionCalls = binary.BigEndian.Uint32(d[:4]), d[4]
			p.SubmissionSaturated = d[5] == 1
			offset += workerSubmissionDetailSize
		}
		// Consume structurally valid diagnostics even when numeric fields are
		// invalid. Metadata cannot turn an acknowledged input into a failure.
		if p.ValidForWorker(value.millis, value.measured) {
			value.applyDetail = &p
		}
	}
	if cursor {
		value.x = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		value.y = int32(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if value.op == opCursorPlace || value.op == opCursorPlaceProbe {
			value.targetX = int32(binary.BigEndian.Uint32(data[offset+8 : offset+12]))
			value.targetY = int32(binary.BigEndian.Uint32(data[offset+12 : offset+16]))
		}
		if value.op == opCursorPlaceProbe {
			p := application.CursorPlacementObservation{Known: data[offset+16], Flags: data[offset+17]}
			if p.Valid() {
				value.observation = &p
			}
		}
	}
	return nil
}
