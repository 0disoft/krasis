// Package inputworker implements the bounded input loop behind a privileged
// local transport. It is not an authentication or service installation API.
package inputworker

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

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
)

type request struct {
	op       operation
	sequence uint64
	binding  [16]byte
	token    control.PressedToken
	x, y     int32
}

func readRequest(reader io.Reader) (request, error) {
	var data [requestSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return request{}, err
	}
	if string(data[:4]) != "KRI1" || data[4] != 1 || data[7] != 0 || binary.BigEndian.Uint64(data[40:]) != 0 {
		return request{}, ErrProtocol
	}
	value := request{op: operation(data[5]), sequence: binary.BigEndian.Uint64(data[8:16])}
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
	case opMove, opWheel:
		if data[6] != 0 {
			return request{}, ErrProtocol
		}
		value.x = int32(binary.BigEndian.Uint32(data[32:36]))
		value.y = int32(binary.BigEndian.Uint32(data[36:40]))
		if value.x == 0 && value.y == 0 {
			return request{}, ErrProtocol
		}
	case opHeartbeat, opClose:
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
	case opMove, opWheel:
		binary.BigEndian.PutUint32(data[32:36], uint32(value.x))
		binary.BigEndian.PutUint32(data[36:40], uint32(value.y))
	case opHeartbeat, opClose:
	default:
		return data, ErrProtocol
	}
	return data, nil
}

type response struct {
	sequence            uint64
	op                  operation
	status              byte
	certainty           byte
	requested, inserted uint16
	timing              [3]byte
	millis              [3]uint16
	measured            uint8
}

func writeResponse(writer io.Writer, value response) error {
	var data [responseSize]byte
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
	n, err := writer.Write(data[:])
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func decodeResponse(data [responseSize]byte) (response, error) {
	if string(data[:4]) != "KRA1" || data[4] < 1 || data[4] > 3 || (data[4] != 3 && data[23] != 0) {
		return response{}, ErrProtocol
	}
	value := response{op: operation(data[5]), status: data[6], certainty: data[7],
		sequence: binary.BigEndian.Uint64(data[8:16]), requested: binary.BigEndian.Uint16(data[16:18]), inserted: binary.BigEndian.Uint16(data[18:20])}
	if data[4] == 3 {
		detail := binary.BigEndian.Uint32(data[20:24])
		if detail == 0 || detail>>30 != 0 {
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
	if value.sequence == 0 || value.op < opPress || value.op > opHeartbeat || value.status > 1 || value.inserted > value.requested {
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
