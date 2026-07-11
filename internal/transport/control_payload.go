package transport

import (
	"errors"
	"fmt"
)

const (
	ControlPayloadVersion uint8 = 1
	controlHeaderSize           = 8
)

var (
	ErrInvalidControlPayload     = errors.New("invalid control payload")
	ErrUnsupportedControlPayload = errors.New("unsupported control payload")
)

type ControlCommandKind uint8

const (
	ControlLeaseRequest ControlCommandKind = 1
	ControlLeaseGrant   ControlCommandKind = 2
	ControlLeaseReject  ControlCommandKind = 3
	ControlLeaseRevoke  ControlCommandKind = 4
)

type ControlCommand struct {
	Kind      ControlCommandKind
	SessionID string
	LeaseID   string
	Epoch     string
}

func EncodeControlPayload(command ControlCommand) ([]byte, error) {
	if err := command.Validate(); err != nil {
		return nil, errors.Join(ErrInvalidControlPayload, err)
	}
	payload := make([]byte, controlHeaderSize+len(command.SessionID)+len(command.LeaseID)+len(command.Epoch))
	payload[0] = ControlPayloadVersion
	payload[1] = byte(command.Kind)
	payload[2] = byte(len(command.SessionID))
	payload[3] = byte(len(command.LeaseID))
	payload[4] = byte(len(command.Epoch))
	offset := controlHeaderSize
	copy(payload[offset:], command.SessionID)
	offset += len(command.SessionID)
	copy(payload[offset:], command.LeaseID)
	offset += len(command.LeaseID)
	copy(payload[offset:], command.Epoch)
	return payload, nil
}

func DecodeControlPayload(payload []byte) (ControlCommand, error) {
	if len(payload) < controlHeaderSize {
		return ControlCommand{}, errors.Join(ErrInvalidControlPayload, errors.New("payload is truncated"))
	}
	if payload[0] != ControlPayloadVersion {
		return ControlCommand{}, fmt.Errorf("%w: control payload version", ErrUnsupportedControlPayload)
	}
	if payload[5] != 0 || payload[6] != 0 || payload[7] != 0 {
		return ControlCommand{}, fmt.Errorf("%w: reserved fields", ErrUnsupportedControlPayload)
	}
	sessionSize := int(payload[2])
	leaseSize := int(payload[3])
	epochSize := int(payload[4])
	if len(payload) != controlHeaderSize+sessionSize+leaseSize+epochSize {
		return ControlCommand{}, errors.Join(ErrInvalidControlPayload, errors.New("identifier lengths do not match payload"))
	}
	offset := controlHeaderSize
	command := ControlCommand{
		Kind:      ControlCommandKind(payload[1]),
		SessionID: string(payload[offset : offset+sessionSize]),
	}
	offset += sessionSize
	command.LeaseID = string(payload[offset : offset+leaseSize])
	offset += leaseSize
	command.Epoch = string(payload[offset : offset+epochSize])
	if err := command.Validate(); err != nil {
		if !command.Kind.Valid() {
			return ControlCommand{}, errors.Join(ErrUnsupportedControlPayload, err)
		}
		return ControlCommand{}, errors.Join(ErrInvalidControlPayload, err)
	}
	return command, nil
}

func (command ControlCommand) Validate() error {
	if !command.Kind.Valid() {
		return fmt.Errorf("control command kind %d is not supported", command.Kind)
	}
	if command.SessionID == "" || command.Epoch == "" {
		return errors.New("session and epoch are required")
	}
	if len(command.SessionID) > maxIdentifierSize || len(command.LeaseID) > maxIdentifierSize || len(command.Epoch) > maxIdentifierSize {
		return errors.New("control identifier is too long")
	}
	switch command.Kind {
	case ControlLeaseRequest, ControlLeaseReject:
		if command.LeaseID != "" {
			return errors.New("lease request or rejection must not include a lease")
		}
	case ControlLeaseGrant, ControlLeaseRevoke:
		if command.LeaseID == "" {
			return errors.New("lease grant or revocation requires a lease")
		}
	}
	return nil
}

func (kind ControlCommandKind) Valid() bool {
	switch kind {
	case ControlLeaseRequest, ControlLeaseGrant, ControlLeaseReject, ControlLeaseRevoke:
		return true
	default:
		return false
	}
}
