package inputworker

import (
	"encoding/binary"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

const workerApplyDetailSize = 12

func encodeWorkerApplyDetail(data []byte, p handoff.WorkerApplyProfile) {
	binary.BigEndian.PutUint32(data[:4], p.Micros[0])
	binary.BigEndian.PutUint32(data[4:8], p.Micros[1])
	data[8], data[9] = p.Measured, p.Saturated
}

// Read the diagnostic and private cursor suffix together. A profiled cursor
// response retains two reads and one write, with no extra request or roundtrip.
func readResponseSuffix(reader io.Reader, version byte, value *response) error {
	var data [workerApplyDetailSize + 8]byte
	size := 0
	if version == 4 {
		size = workerApplyDetailSize
	}
	cursor := (value.op == opCursorRead || value.op == opMoveRead) && value.status == 0
	if cursor {
		size += 8
	}
	if size == 0 {
		return nil
	}
	if _, err := io.ReadFull(reader, data[:size]); err != nil {
		return ErrProtocol
	}
	offset := 0
	if version == 4 {
		if data[10] != 0 || data[11] != 0 || value.op == opHeartbeat || value.op == opClose {
			return ErrProtocol
		}
		p := handoff.WorkerApplyProfile{Micros: [2]uint32{binary.BigEndian.Uint32(data[:4]), binary.BigEndian.Uint32(data[4:8])}, Measured: data[8], Saturated: data[9]}
		// Consume structurally valid diagnostics even when numeric fields are
		// invalid. Metadata cannot turn an acknowledged input into a failure.
		if p.Valid() {
			value.applyDetail = &p
		}
		offset = workerApplyDetailSize
	}
	if cursor {
		value.x = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		value.y = int32(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
	}
	return nil
}
