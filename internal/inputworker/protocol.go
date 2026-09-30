// Package inputworker implements the bounded input loop behind a privileged
// local transport. It is not an authentication or service installation API.
package inputworker

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/0disoft/krasis/internal/control"
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
}

func writeResponse(writer io.Writer, value response) error {
	var data [responseSize]byte
	copy(data[:4], "KRA1")
	data[4], data[5], data[6], data[7] = 1, byte(value.op), value.status, value.certainty
	binary.BigEndian.PutUint64(data[8:16], value.sequence)
	binary.BigEndian.PutUint16(data[16:18], value.requested)
	binary.BigEndian.PutUint16(data[18:20], value.inserted)
	n, err := writer.Write(data[:])
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func decodeResponse(data [responseSize]byte) (response, error) {
	if string(data[:4]) != "KRA1" || data[4] != 1 || binary.BigEndian.Uint32(data[20:]) != 0 {
		return response{}, ErrProtocol
	}
	value := response{op: operation(data[5]), status: data[6], certainty: data[7],
		sequence: binary.BigEndian.Uint64(data[8:16]), requested: binary.BigEndian.Uint16(data[16:18]), inserted: binary.BigEndian.Uint16(data[18:20])}
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
