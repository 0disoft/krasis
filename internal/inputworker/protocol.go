// Package inputworker implements the bounded input loop behind a privileged
// local transport. It is not an authentication or service installation API.
package inputworker

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

const (
	requestSize  = 48
	responseSize = 24
)

var ErrProtocol = errors.New("input worker protocol rejected")

type operation byte

const (
	opPress operation = iota + 1
	opRelease
	opMove
	opWheel
	opHeartbeat
	opClose
	opCursorRead
	opCursorSet
	opMoveRead
	opCursorPlace
	opCursorPlaceProbe
)

type request struct {
	op               operation
	sequence         uint64
	binding          [16]byte
	token            control.PressedToken
	x, y             int32
	applyDetail      bool
	submissionDetail bool
}

func readRequest(reader io.Reader) (request, error) {
	var data [requestSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return request{}, err
	}
	if string(data[:4]) != "KRI1" || (data[4] != 1 && data[4] != 2) || data[4] == 1 && data[7] != 0 || data[4] == 2 && data[7] != 1 && data[7] != 3 || binary.BigEndian.Uint64(data[40:]) != 0 {
		return request{}, ErrProtocol
	}
	value := request{op: operation(data[5]), sequence: binary.BigEndian.Uint64(data[8:16])}
	value.applyDetail = data[4] == 2
	value.submissionDetail = value.applyDetail && data[7] == 3
	if value.applyDetail && (value.op == opHeartbeat || value.op == opClose) {
		return request{}, ErrProtocol
	}
	copy(value.binding[:], data[16:32])
	switch value.op {
	case opPress, opRelease:
		if binary.BigEndian.Uint32(data[36:40]) != 0 {
			return request{}, ErrProtocol
		}
		switch data[6] {
		case 1:
			value.token.Kind = control.PressedKeyboardKey
		case 2:
			value.token.Kind = control.PressedMouseButton
		default:
			return request{}, ErrProtocol
		}
		value.token.Code = binary.BigEndian.Uint32(data[32:36])
		if value.token.Validate() != nil {
			return request{}, ErrProtocol
		}
	case opMove, opWheel, opCursorSet, opMoveRead, opCursorPlace, opCursorPlaceProbe:
		if data[6] != 0 {
			return request{}, ErrProtocol
		}
		value.x = int32(binary.BigEndian.Uint32(data[32:36]))
		value.y = int32(binary.BigEndian.Uint32(data[36:40]))
		if value.op != opCursorSet && value.op != opCursorPlace && value.op != opCursorPlaceProbe && value.x == 0 && value.y == 0 {
			return request{}, ErrProtocol
		}
	case opHeartbeat, opClose, opCursorRead:
		if data[6] != 0 || binary.BigEndian.Uint64(data[32:40]) != 0 {
			return request{}, ErrProtocol
		}
	default:
		return request{}, ErrProtocol
	}
	return value, nil
}

func encodeRequest(value request) ([requestSize]byte, error) {
	var data [requestSize]byte
	copy(data[:4], "KRI1")
	data[4], data[5] = 1, byte(value.op)
	if value.submissionDetail && !value.applyDetail {
		return data, ErrProtocol
	}
	if value.applyDetail {
		if value.op == opHeartbeat || value.op == opClose {
			return data, ErrProtocol
		}
		data[4], data[7] = 2, 1
		if value.submissionDetail {
			data[7] = 3
		}
	}
	binary.BigEndian.PutUint64(data[8:16], value.sequence)
	copy(data[16:32], value.binding[:])
	switch value.op {
	case opPress, opRelease:
		switch value.token.Kind {
		case control.PressedKeyboardKey:
			data[6] = 1
		case control.PressedMouseButton:
			data[6] = 2
		default:
			return data, ErrProtocol
		}
		binary.BigEndian.PutUint32(data[32:36], value.token.Code)
	case opMove, opWheel, opCursorSet, opMoveRead, opCursorPlace, opCursorPlaceProbe:
		binary.BigEndian.PutUint32(data[32:36], uint32(value.x))
		binary.BigEndian.PutUint32(data[36:40], uint32(value.y))
	case opHeartbeat, opClose, opCursorRead:
	default:
		return data, ErrProtocol
	}
	return data, nil
}

type response struct {
	observation         *application.CursorPlacementObservation
	x, y                int32
	targetX, targetY    int32
	sequence            uint64
	op                  operation
	status              byte
	certainty           byte
	requested, inserted uint16
	timing              [3]byte
	millis              [3]uint16
	measured            uint8
	applyDetail         *handoff.WorkerApplyProfile
}

func writeResponse(writer io.Writer, value response) error {
	var data [responseSize + workerApplyDetailSize + workerSubmissionDetailSize + 18]byte
	size := responseSize
	copy(data[:4], "KRA1")
	data[4], data[5], data[6], data[7] = 1, byte(value.op), value.status, value.certainty
	binary.BigEndian.PutUint64(data[8:16], value.sequence)
	binary.BigEndian.PutUint16(data[16:18], value.requested)
	binary.BigEndian.PutUint16(data[18:20], value.inserted)
	if value.timing != ([3]byte{}) {
		for _, bucket := range value.timing {
			if bucket > 5 {
				return ErrProtocol
			}
		}
		data[4] = 2
		copy(data[20:23], value.timing[:])
	}
	if value.measured != 0 || value.millis != ([3]uint16{}) {
		if !handoff.ValidWorkerDetail(value.timing, value.millis, value.measured) {
			return ErrProtocol
		}
		// Three ten-bit fields fit in the existing four diagnostic bytes:
		// 0 is absent, 1 is measured zero, 1023 is >=1022ms. This adds no
		// bytes, reads, writes or round trips to the serialized input path.
		var detail uint32
		for i, n := range value.millis {
			if value.measured&(1<<i) != 0 {
				detail |= uint32(n+1) << (i * 10)
			}
		}
		data[4] = 3
		binary.BigEndian.PutUint32(data[20:24], detail)
	}
	if value.applyDetail != nil && value.applyDetail.ValidForWorker(value.millis, value.measured) && (value.measured != 0 || value.timing == ([3]byte{})) {
		data[4] = 4
		encodeWorkerApplyDetail(data[responseSize:responseSize+workerApplyDetailSize], *value.applyDetail)
		size += workerApplyDetailSize
		if value.applyDetail.SubmissionCalls != 0 {
			data[4] = 5
			encodeWorkerSubmission(data[size:size+workerSubmissionDetailSize], *value.applyDetail)
			size += workerSubmissionDetailSize
		}
	}
	if (value.op == opCursorRead || value.op == opMoveRead || value.op == opCursorPlace || value.op == opCursorPlaceProbe) && value.status == 0 {
		// Cursor snapshots add eight private bytes in the same pipe write.
		// Other ACKs retain their existing size; coordinates never enter logs.
		binary.BigEndian.PutUint32(data[size:size+4], uint32(value.x))
		binary.BigEndian.PutUint32(data[size+4:], uint32(value.y))
		size += 8
		if value.op == opCursorPlace || value.op == opCursorPlaceProbe {
			binary.BigEndian.PutUint32(data[size:size+4], uint32(value.targetX))
			binary.BigEndian.PutUint32(data[size+4:], uint32(value.targetY))
			size += 8
		}
		if value.op == opCursorPlaceProbe {
			if value.observation != nil && value.observation.Valid() {
				data[size], data[size+1] = value.observation.Known, value.observation.Flags
			}
			size += 2
		}
	}
	n, err := writer.Write(data[:size])
	if err == nil && n != size {
		return io.ErrShortWrite
	}
	return err
}

func decodeResponse(data [responseSize]byte) (response, error) {
	if string(data[:4]) != "KRA1" || data[4] < 1 || data[4] > 5 || (data[4] < 3 && data[23] != 0) {
		return response{}, ErrProtocol
	}
	value := response{op: operation(data[5]), status: data[6], certainty: data[7],
		sequence: binary.BigEndian.Uint64(data[8:16]), requested: binary.BigEndian.Uint16(data[16:18]), inserted: binary.BigEndian.Uint16(data[18:20])}
	if data[4] >= 3 {
		detail := binary.BigEndian.Uint32(data[20:24])
		if data[4] == 3 && detail == 0 || detail>>30 != 0 {
			return response{}, ErrProtocol
		}
		for i := range value.millis {
			encoded := (detail >> (i * 10)) & 1023
			if encoded != 0 {
				value.measured |= 1 << i
				value.millis[i] = uint16(encoded - 1)
				value.timing[i] = timingBucket(time.Duration(encoded-1) * time.Millisecond)
			}
		}
	} else {
		copy(value.timing[:], data[20:23])
	}
	if data[4] == 1 && value.timing != ([3]byte{}) || data[4] == 2 && value.timing == ([3]byte{}) {
		return response{}, ErrProtocol
	}
	for _, bucket := range value.timing {
		if bucket > 5 {
			return response{}, ErrProtocol
		}
	}
	if value.sequence == 0 || value.op < opPress || value.op > opCursorPlaceProbe || value.op == opClose || value.status > 1 || value.inserted > value.requested {
		return response{}, ErrProtocol
	}
	if value.status == 0 {
		if value.certainty != 0 || value.requested != 0 || value.inserted != 0 {
			return response{}, ErrProtocol
		}
	} else {
		switch value.certainty {
		case 1:
			if value.inserted != 0 {
				return response{}, ErrProtocol
			}
		case 2:
			if value.inserted == 0 || value.inserted == value.requested {
				return response{}, ErrProtocol
			}
		case 3:
		default:
			return response{}, ErrProtocol
		}
	}
	return value, nil
}
