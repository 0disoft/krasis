package transport

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

const (
	InputPayloadVersion    uint8 = 1
	inputPayloadHeaderSize       = 28
	maxIdentifierSize            = 255
)

var (
	ErrInvalidInputPayload     = errors.New("invalid input payload")
	ErrUnsupportedInputPayload = errors.New("unsupported input payload")
)

func EncodeInputPayload(envelope control.InputEnvelope, action control.InputAction) ([]byte, error) {
	if err := validateInputPayload(envelope, action); err != nil {
		return nil, err
	}
	sessionSize := len(envelope.SessionID)
	epochSize := len(envelope.Epoch)
	payload := make([]byte, inputPayloadHeaderSize+sessionSize+epochSize)
	payload[0] = InputPayloadVersion
	payload[1] = encodeInputActionKind(action.Kind)
	binary.BigEndian.PutUint64(payload[4:12], envelope.Sequence)
	binary.BigEndian.PutUint32(payload[12:16], action.Code)
	binary.BigEndian.PutUint32(payload[16:20], uint32(action.DeltaX))
	binary.BigEndian.PutUint32(payload[20:24], uint32(action.DeltaY))
	payload[24] = byte(sessionSize)
	payload[25] = byte(epochSize)
	copy(payload[inputPayloadHeaderSize:inputPayloadHeaderSize+sessionSize], envelope.SessionID)
	copy(payload[inputPayloadHeaderSize+sessionSize:], envelope.Epoch)
	return payload, nil
}

func DecodeInputPayload(payload []byte) (control.InputEnvelope, control.InputAction, error) {
	return DecodeInputPayloadReusingIdentifiers(payload, control.InputEnvelope{})
}

// DecodeInputPayloadReusingIdentifiers reuses previous identifiers only when
// their bytes match exactly. Changed identifiers are copied from payload; all
// fields are validated normally, and sequence is always read from the payload.
// The returned strings never borrow mutable payload storage.
func DecodeInputPayloadReusingIdentifiers(payload []byte, previous control.InputEnvelope) (control.InputEnvelope, control.InputAction, error) {
	if len(payload) < inputPayloadHeaderSize {
		return control.InputEnvelope{}, control.InputAction{}, errors.Join(ErrInvalidInputPayload, errors.New("payload is truncated"))
	}
	if payload[0] != InputPayloadVersion {
		return control.InputEnvelope{}, control.InputAction{}, fmt.Errorf("%w: input payload version", ErrUnsupportedInputPayload)
	}
	if binary.BigEndian.Uint16(payload[2:4]) != 0 || binary.BigEndian.Uint16(payload[26:28]) != 0 {
		return control.InputEnvelope{}, control.InputAction{}, fmt.Errorf("%w: reserved fields", ErrUnsupportedInputPayload)
	}
	kind, ok := decodeInputActionKind(payload[1])
	if !ok {
		return control.InputEnvelope{}, control.InputAction{}, fmt.Errorf("%w: input action kind", ErrUnsupportedInputPayload)
	}
	sessionSize := int(payload[24])
	epochSize := int(payload[25])
	if sessionSize == 0 || epochSize == 0 || len(payload) != inputPayloadHeaderSize+sessionSize+epochSize {
		return control.InputEnvelope{}, control.InputAction{}, errors.Join(ErrInvalidInputPayload, errors.New("identifier lengths do not match payload"))
	}
	envelope := control.InputEnvelope{
		SessionID: reuseInputIdentifier(payload[inputPayloadHeaderSize:inputPayloadHeaderSize+sessionSize], previous.SessionID),
		Epoch:     reuseInputIdentifier(payload[inputPayloadHeaderSize+sessionSize:], previous.Epoch),
		Sequence:  binary.BigEndian.Uint64(payload[4:12]),
	}
	action := control.InputAction{
		Kind:   kind,
		Code:   binary.BigEndian.Uint32(payload[12:16]),
		DeltaX: int32(binary.BigEndian.Uint32(payload[16:20])),
		DeltaY: int32(binary.BigEndian.Uint32(payload[20:24])),
	}
	if err := validateInputPayload(envelope, action); err != nil {
		return control.InputEnvelope{}, control.InputAction{}, err
	}
	return envelope, action, nil
}

func reuseInputIdentifier(payload []byte, previous string) string {
	if string(payload) == previous {
		return previous
	}
	return string(payload)
}

func validateInputPayload(envelope control.InputEnvelope, action control.InputAction) error {
	if envelope.SessionID == "" || envelope.Epoch == "" || envelope.Sequence == 0 {
		return errors.Join(ErrInvalidInputPayload, errors.New("session, epoch, and sequence are required"))
	}
	if len(envelope.SessionID) > maxIdentifierSize || len(envelope.Epoch) > maxIdentifierSize {
		return errors.Join(ErrInvalidInputPayload, errors.New("session or epoch identifier is too long"))
	}
	if err := action.Validate(); err != nil {
		return errors.Join(ErrInvalidInputPayload, err)
	}
	return nil
}

func encodeInputActionKind(kind control.InputActionKind) byte {
	switch kind {
	case control.InputKeyDown:
		return 1
	case control.InputKeyUp:
		return 2
	case control.InputButtonDown:
		return 3
	case control.InputButtonUp:
		return 4
	case control.InputPointerMove:
		return 5
	case control.InputWheel:
		return 6
	default:
		return 0
	}
}

func decodeInputActionKind(encoded byte) (control.InputActionKind, bool) {
	switch encoded {
	case 1:
		return control.InputKeyDown, true
	case 2:
		return control.InputKeyUp, true
	case 3:
		return control.InputButtonDown, true
	case 4:
		return control.InputButtonUp, true
	case 5:
		return control.InputPointerMove, true
	case 6:
		return control.InputWheel, true
	default:
		return "", false
	}
}
