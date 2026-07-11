package transport

import (
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestInputPayloadRoundTrip(t *testing.T) {
	actions := []control.InputAction{
		{Kind: control.InputKeyDown, Code: 65},
		{Kind: control.InputKeyUp, Code: 65},
		{Kind: control.InputButtonDown, Code: 1},
		{Kind: control.InputButtonUp, Code: 1},
		{Kind: control.InputPointerMove, DeltaX: -3, DeltaY: 4},
		{Kind: control.InputWheel, DeltaX: -120, DeltaY: 120},
	}
	for index, action := range actions {
		envelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: uint64(index + 1)}
		payload, err := EncodeInputPayload(envelope, action)
		if err != nil {
			t.Fatalf("EncodeInputPayload(%s) error = %v", action.Kind, err)
		}
		gotEnvelope, gotAction, err := DecodeInputPayload(payload)
		if err != nil {
			t.Fatalf("DecodeInputPayload(%s) error = %v", action.Kind, err)
		}
		if !reflect.DeepEqual(gotEnvelope, envelope) || !reflect.DeepEqual(gotAction, action) {
			t.Fatalf("round trip = (%+v, %+v), want (%+v, %+v)", gotEnvelope, gotAction, envelope, action)
		}
	}
}

func TestInputPayloadEncoderRejectsInvalidValues(t *testing.T) {
	validEnvelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}
	validAction := control.InputAction{Kind: control.InputKeyDown, Code: 65}
	tests := []struct {
		name     string
		envelope control.InputEnvelope
		action   control.InputAction
	}{
		{name: "empty session", envelope: control.InputEnvelope{Epoch: "epoch-1", Sequence: 1}, action: validAction},
		{name: "zero sequence", envelope: control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1"}, action: validAction},
		{name: "long epoch", envelope: control.InputEnvelope{SessionID: "session-1", Epoch: strings.Repeat("e", maxIdentifierSize+1), Sequence: 1}, action: validAction},
		{name: "invalid action", envelope: validEnvelope, action: control.InputAction{Kind: control.InputPointerMove}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := EncodeInputPayload(test.envelope, test.action)
			if !errors.Is(err, ErrInvalidInputPayload) {
				t.Fatalf("error = %v, want ErrInvalidInputPayload", err)
			}
		})
	}
}

func TestInputPayloadDecoderRejectsMalformedPayload(t *testing.T) {
	envelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}
	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	valid, err := EncodeInputPayload(envelope, action)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		want   error
	}{
		{name: "truncated", mutate: func(payload []byte) []byte { return payload[:inputPayloadHeaderSize-1] }, want: ErrInvalidInputPayload},
		{name: "version", mutate: func(payload []byte) []byte { payload[0]++; return payload }, want: ErrUnsupportedInputPayload},
		{name: "kind", mutate: func(payload []byte) []byte { payload[1] = 99; return payload }, want: ErrUnsupportedInputPayload},
		{name: "flags", mutate: func(payload []byte) []byte { payload[2] = 1; return payload }, want: ErrUnsupportedInputPayload},
		{name: "reserved", mutate: func(payload []byte) []byte { payload[26] = 1; return payload }, want: ErrUnsupportedInputPayload},
		{name: "zero sequence", mutate: func(payload []byte) []byte { binary.BigEndian.PutUint64(payload[4:12], 0); return payload }, want: ErrInvalidInputPayload},
		{name: "trailing", mutate: func(payload []byte) []byte { return append(payload, 0) }, want: ErrInvalidInputPayload},
		{name: "invalid action fields", mutate: func(payload []byte) []byte { binary.BigEndian.PutUint32(payload[12:16], 1); return payload }, want: ErrInvalidInputPayload},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := test.mutate(append([]byte(nil), valid...))
			_, _, err := DecodeInputPayload(payload)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestInputPayloadFitsBoundedFrame(t *testing.T) {
	payload, err := EncodeInputPayload(
		control.InputEnvelope{
			SessionID: strings.Repeat("s", maxIdentifierSize),
			Epoch:     strings.Repeat("e", maxIdentifierSize),
			Sequence:  1,
		},
		control.InputAction{Kind: control.InputWheel, DeltaY: 120},
	)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	if len(payload) > int(MaxPayloadSize) {
		t.Fatalf("maximum input payload = %d, frame maximum = %d", len(payload), MaxPayloadSize)
	}
}
