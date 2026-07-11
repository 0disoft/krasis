package transport

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	HeartbeatPayloadVersion uint8 = 1
	heartbeatHeaderSize           = 4
)

var (
	ErrInvalidHeartbeatPayload     = errors.New("invalid heartbeat payload")
	ErrUnsupportedHeartbeatPayload = errors.New("unsupported heartbeat payload")
)

func EncodeHeartbeatPayload(sessionID string) ([]byte, error) {
	if sessionID == "" || len(sessionID) > maxIdentifierSize {
		return nil, errors.Join(ErrInvalidHeartbeatPayload, errors.New("session identifier length is invalid"))
	}
	payload := make([]byte, heartbeatHeaderSize+len(sessionID))
	payload[0] = HeartbeatPayloadVersion
	payload[1] = byte(len(sessionID))
	copy(payload[heartbeatHeaderSize:], sessionID)
	return payload, nil
}

func DecodeHeartbeatPayload(payload []byte) (string, error) {
	if len(payload) < heartbeatHeaderSize {
		return "", errors.Join(ErrInvalidHeartbeatPayload, errors.New("payload is truncated"))
	}
	if payload[0] != HeartbeatPayloadVersion {
		return "", fmt.Errorf("%w: heartbeat payload version", ErrUnsupportedHeartbeatPayload)
	}
	if binary.BigEndian.Uint16(payload[2:4]) != 0 {
		return "", fmt.Errorf("%w: reserved fields", ErrUnsupportedHeartbeatPayload)
	}
	sessionSize := int(payload[1])
	if sessionSize == 0 || len(payload) != heartbeatHeaderSize+sessionSize {
		return "", errors.Join(ErrInvalidHeartbeatPayload, errors.New("session identifier length does not match payload"))
	}
	return string(payload[heartbeatHeaderSize:]), nil
}
