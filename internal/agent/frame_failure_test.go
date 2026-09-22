package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

func TestMalformedFramesKeepSpecificReasonThroughSessionCleanup(t *testing.T) {
	packet := func(kind transport.MessageKind, payload []byte) []byte {
		var buffer bytes.Buffer
		writer, _ := transport.NewWriter(&buffer)
		if err := writer.WriteFrame(transport.Frame{Kind: kind, Payload: payload}); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	base := packet(transport.MessageInput, []byte{1, 2})
	cases := []struct {
		code string
		data []byte
	}{
		{"remote_eof", nil},
		{"frame_header_truncated", base[:3]},
		{"frame_payload_truncated", base[:len(base)-1]},
		{"frame_payload_truncated", base[:12]},
	}
	for _, mutation := range []struct {
		code   string
		offset int
	}{{"magic_invalid", 0}, {"version_unsupported", 4}, {"kind_unsupported", 5}, {"flags_unsupported", 6}} {
		data := append([]byte(nil), base...)
		data[mutation.offset] = 255
		cases = append(cases, struct {
			code string
			data []byte
		}{"frame_" + mutation.code, data})
	}
	oversized := append([]byte(nil), base...)
	binary.BigEndian.PutUint32(oversized[8:12], transport.MaxPayloadSize+1)
	cases = append(cases, struct {
		code string
		data []byte
	}{"frame_payload_oversized", oversized})
	controlPayload, _ := transport.EncodeControlPayload(transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "s", Epoch: "e", Edge: control.ScreenEdgeRight})
	inputPayload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
	heartbeatPayload, _ := transport.EncodeHeartbeatPayload("s")
	for _, entry := range []struct {
		name    string
		kind    transport.MessageKind
		payload []byte
	}{{"control", transport.MessageControl, controlPayload}, {"input", transport.MessageInput, inputPayload}, {"heartbeat", transport.MessageHeartbeat, heartbeatPayload}} {
		cases = append(cases, struct {
			code string
			data []byte
		}{"frame_" + entry.name + "_invalid", packet(entry.kind, nil)})
		entry.payload[0] = 255
		cases = append(cases, struct {
			code string
			data []byte
		}{"frame_" + entry.name + "_unsupported", packet(entry.kind, entry.payload)})
	}
	for _, fixture := range cases {
		t.Run(fixture.code, func(t *testing.T) {
			client, server := openConnectionPair(t)
			defer client.Close()
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			written := make(chan struct{})
			go func() { defer close(written); _, _ = client.Conn.Write(fixture.data); _ = client.Close() }()
			input := newTestInput(false)
			config := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10})
			result, err := RunSession(ctx, config, Dependencies{Connection: server, Hook: &idleHook{}, Input: input, Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
			<-written
			descriptor := DescribeSessionFailure(err)
			if got := productlog.FaultCode(descriptor.Component, descriptor.Phase, descriptor.Code); got != "fault_"+fixture.code {
				t.Fatalf("failure reason: %s, %v", got, err)
			}
			if result.FinalState != control.StateLocal || result.Recovery.PressedLedgerEmpty != RecoveryConfirmed || len(input.moves) != 0 {
				t.Fatalf("unsafe malformed-frame cleanup: %+v", result)
			}
		})
	}
}
