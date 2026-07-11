package transport

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestControlPayloadRoundTrip(t *testing.T) {
	commands := []ControlCommand{
		{Kind: ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight},
		{Kind: ControlLeaseGrant, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"},
		{Kind: ControlLeaseReject, SessionID: "session-1", Epoch: "epoch-1"},
		{Kind: ControlLeaseRevoke, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"},
	}
	for _, command := range commands {
		payload, err := EncodeControlPayload(command)
		if err != nil {
			t.Fatalf("EncodeControlPayload(%d) error = %v", command.Kind, err)
		}
		got, err := DecodeControlPayload(payload)
		if err != nil {
			t.Fatalf("DecodeControlPayload(%d) error = %v", command.Kind, err)
		}
		if !reflect.DeepEqual(got, command) {
			t.Fatalf("round trip = %+v, want %+v", got, command)
		}
	}
}

func TestControlPayloadRejectsInvalidCommands(t *testing.T) {
	tests := []ControlCommand{
		{},
		{Kind: ControlLeaseRequest, SessionID: "session-1"},
		{Kind: ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1"},
		{Kind: ControlLeaseRequest, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight},
		{Kind: ControlLeaseGrant, SessionID: "session-1", Epoch: "epoch-1"},
		{Kind: ControlLeaseReject, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"},
		{Kind: ControlLeaseRevoke, SessionID: "session-1", Epoch: "epoch-1"},
		{Kind: ControlLeaseRequest, SessionID: strings.Repeat("s", maxIdentifierSize+1), Epoch: "epoch-1", Edge: control.ScreenEdgeRight},
		{Kind: ControlLeaseGrant, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1", Edge: control.ScreenEdgeLeft},
	}
	for _, command := range tests {
		if _, err := EncodeControlPayload(command); !errors.Is(err, ErrInvalidControlPayload) {
			t.Fatalf("EncodeControlPayload(%+v) error = %v", command, err)
		}
	}
}

func TestControlPayloadRejectsMalformedPayload(t *testing.T) {
	valid, err := EncodeControlPayload(ControlCommand{
		Kind:      ControlLeaseGrant,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	})
	if err != nil {
		t.Fatalf("EncodeControlPayload() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		want   error
	}{
		{name: "truncated", mutate: func(payload []byte) []byte { return payload[:controlHeaderSize-1] }, want: ErrInvalidControlPayload},
		{name: "version", mutate: func(payload []byte) []byte { payload[0]++; return payload }, want: ErrUnsupportedControlPayload},
		{name: "kind", mutate: func(payload []byte) []byte { payload[1] = 99; return payload }, want: ErrUnsupportedControlPayload},
		{name: "edge", mutate: func(payload []byte) []byte { payload[5] = 99; return payload }, want: ErrUnsupportedControlPayload},
		{name: "reserved", mutate: func(payload []byte) []byte { payload[6] = 1; return payload }, want: ErrUnsupportedControlPayload},
		{name: "missing lease", mutate: func(payload []byte) []byte { payload[3] = 0; return payload }, want: ErrInvalidControlPayload},
		{name: "trailing", mutate: func(payload []byte) []byte { return append(payload, 0) }, want: ErrInvalidControlPayload},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := test.mutate(append([]byte(nil), valid...))
			_, err := DecodeControlPayload(payload)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestControlPayloadFitsBoundedFrame(t *testing.T) {
	payload, err := EncodeControlPayload(ControlCommand{
		Kind:      ControlLeaseGrant,
		SessionID: strings.Repeat("s", maxIdentifierSize),
		LeaseID:   strings.Repeat("l", maxIdentifierSize),
		Epoch:     strings.Repeat("e", maxIdentifierSize),
	})
	if err != nil {
		t.Fatalf("EncodeControlPayload() error = %v", err)
	}
	if len(payload) > int(MaxPayloadSize) {
		t.Fatalf("maximum control payload = %d, frame maximum = %d", len(payload), MaxPayloadSize)
	}
}
