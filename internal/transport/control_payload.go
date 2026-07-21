package transport

import (
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

const (
	ControlPayloadVersion uint8 = 3
	controlHeaderSize           = 8
	controlEdgeNone       uint8 = 0
	controlEdgeLeft       uint8 = 1
	controlEdgeRight      uint8 = 2
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
	ControlLeaseCommit  ControlCommandKind = 5
	ControlLeaseAbort   ControlCommandKind = 6
)

type ControlCommand struct {
	Kind      ControlCommandKind
	SessionID string
	LeaseID   string
	Epoch     string
	Edge      control.ScreenEdge
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
	payload[5] = encodeControlEdge(command.Edge)
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
	if payload[6] != 0 || payload[7] != 0 {
		return ControlCommand{}, fmt.Errorf("%w: reserved fields", ErrUnsupportedControlPayload)
	}
	edge, err := decodeControlEdge(payload[5])
	if err != nil {
		return ControlCommand{}, err
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
		Edge:      edge,
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
	case ControlLeaseRequest:
		if command.LeaseID != "" || (command.Edge != control.ScreenEdgeLeft && command.Edge != control.ScreenEdgeRight) {
			return errors.New("lease request requires a LEFT or RIGHT edge and no lease")
		}
	case ControlLeaseReject:
		if command.LeaseID != "" {
			return errors.New("lease rejection must not include a lease")
		}
	case ControlLeaseGrant, ControlLeaseRevoke, ControlLeaseCommit, ControlLeaseAbort:
		if command.LeaseID == "" {
			return errors.New("lease grant, commit, abort, or revocation requires a lease")
		}
	}
	if command.Kind != ControlLeaseRequest && command.Edge != "" {
		return errors.New("only lease requests may include an edge")
	}
	return nil
}

func encodeControlEdge(edge control.ScreenEdge) uint8 {
	switch edge {
	case control.ScreenEdgeLeft:
		return controlEdgeLeft
	case control.ScreenEdgeRight:
		return controlEdgeRight
	default:
		return controlEdgeNone
	}
}

func decodeControlEdge(value uint8) (control.ScreenEdge, error) {
	switch value {
	case controlEdgeNone:
		return "", nil
	case controlEdgeLeft:
		return control.ScreenEdgeLeft, nil
	case controlEdgeRight:
		return control.ScreenEdgeRight, nil
	default:
		return "", fmt.Errorf("%w: control edge", ErrUnsupportedControlPayload)
	}
}

func (kind ControlCommandKind) Valid() bool {
	switch kind {
	case ControlLeaseRequest, ControlLeaseGrant, ControlLeaseReject, ControlLeaseRevoke, ControlLeaseCommit, ControlLeaseAbort:
		return true
	default:
		return false
	}
}
