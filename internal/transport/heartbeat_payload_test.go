package transport

import (
	"errors"
	"strings"
	"testing"
)

func TestHeartbeatPayloadRoundTrip(t *testing.T) {
	payload, err := EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	sessionID, err := DecodeHeartbeatPayload(payload)
	if err != nil {
		t.Fatalf("DecodeHeartbeatPayload() error = %v", err)
	}
	if sessionID != "session-1" {
		t.Fatalf("session ID = %q", sessionID)
	}
}

func TestHeartbeatPayloadRejectsInvalidIdentifier(t *testing.T) {
	for _, sessionID := range []string{"", strings.Repeat("s", maxIdentifierSize+1)} {
		if _, err := EncodeHeartbeatPayload(sessionID); !errors.Is(err, ErrInvalidHeartbeatPayload) {
			t.Fatalf("EncodeHeartbeatPayload(%d bytes) error = %v", len(sessionID), err)
		}
	}
}

func TestHeartbeatPayloadRejectsMalformedPayload(t *testing.T) {
	valid, err := EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		want   error
	}{
		{name: "truncated", mutate: func(payload []byte) []byte { return payload[:heartbeatHeaderSize-1] }, want: ErrInvalidHeartbeatPayload},
		{name: "version", mutate: func(payload []byte) []byte { payload[0]++; return payload }, want: ErrUnsupportedHeartbeatPayload},
		{name: "reserved", mutate: func(payload []byte) []byte { payload[2] = 1; return payload }, want: ErrUnsupportedHeartbeatPayload},
		{name: "zero length", mutate: func(payload []byte) []byte { payload[1] = 0; return payload }, want: ErrInvalidHeartbeatPayload},
		{name: "wrong length", mutate: func(payload []byte) []byte { payload[1]--; return payload }, want: ErrInvalidHeartbeatPayload},
		{name: "trailing", mutate: func(payload []byte) []byte { return append(payload, 0) }, want: ErrInvalidHeartbeatPayload},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := test.mutate(append([]byte(nil), valid...))
			_, err := DecodeHeartbeatPayload(payload)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
