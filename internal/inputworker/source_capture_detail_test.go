package inputworker

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestSourceCaptureDetailUsesOnlyExactFixedSizeActivationReply(t *testing.T) {
	for _, status := range []byte{0, 1} {
		var detail handoff.CaptureNative
		for i := range detail {
			detail[i] = byte(i%5 + 1)
		}
		exact := handoff.CaptureNativeExact{QueueToHandleMicros: 7000, ForegroundRequestMicros: 210000, ForegroundVerifyMicros: 0, Measured: 1<<handoff.CaptureExactQueueToHandle | 1<<handoff.CaptureExactForegroundRequest | 1<<handoff.CaptureExactForegroundVerify}
		p := sourcePacket{kind: sourceReply, op: sourceActivate, status: status, sequence: 3, reply: 2, binding: Binding{1}, capture: detail, captureExact: exact}
		wire, err := encodeSourcePacket(p)
		if err != nil || len(wire) != 64 {
			t.Fatal("capture detail changed reply framing", err)
		}
		got, err := readSourcePacket(bytes.NewReader(wire[:]))
		if err != nil || got != p {
			t.Fatal("success/rejection lost owner detail or identity", got, err)
		}
		badKind := wire
		badKind[7] = 1
		if _, err := readSourcePacket(bytes.NewReader(badKind[:])); err != ErrProtocol {
			t.Fatal("activation reply accepted an action kind", err)
		}
		for _, bits := range []uint64{6, 7, 1 << (3 * len(detail))} {
			bad := wire
			binary.BigEndian.PutUint32(bad[52:56], uint32(bits>>32))
			binary.BigEndian.PutUint32(bad[60:64], uint32(bits))
			if _, err := readSourcePacket(bytes.NewReader(bad[:])); err != ErrProtocol {
				t.Fatal("invalid native bucket or unused bits accepted", bits, err)
			}
		}
		for op := sourceStart; op <= sourceCursorRestore; op++ {
			if op == sourceActivate {
				continue
			}
			bad := wire
			bad[5] = op
			if _, err := readSourcePacket(bytes.NewReader(bad[:])); err != ErrProtocol {
				t.Fatal("owner timing escaped activation reply", op, err)
			}
		}
		for op := sourceStart; op <= sourceCursorRestore; op++ {
			if op == sourceActivate {
				continue
			}
			nonActivation := p
			nonActivation.op = op
			if _, err := encodeSourcePacket(nonActivation); err != ErrProtocol {
				t.Fatal("exact owner payload escaped activation reply", op, err)
			}
		}
		legacy := p
		legacy.captureExact = handoff.CaptureNativeExact{}
		legacyWire, err := encodeSourcePacket(legacy)
		if err != nil {
			t.Fatal("legacy all-zero exact detail no longer encodes", err)
		}
		if decoded, err := readSourcePacket(bytes.NewReader(legacyWire[:])); err != nil || decoded.captureExact != (handoff.CaptureNativeExact{}) {
			t.Fatal("legacy all-zero detail was not unobserved", decoded.captureExact, err)
		}
		badExact := wire
		binary.BigEndian.PutUint32(badExact[40:44], uint32(handoff.Window.Microseconds())+2)
		if _, err := readSourcePacket(bytes.NewReader(badExact[:])); err != ErrProtocol {
			t.Fatal("exact owner timing outside the bounded window was accepted", err)
		}
		for n := 0; n < len(wire); n++ {
			if _, err := readSourcePacket(bytes.NewReader(wire[:n])); err == nil {
				t.Fatal("truncated activation reply accepted", n)
			}
		}
		p.kind = sourceCommand
		p.reply = 0
		if _, err := encodeSourcePacket(p); err != ErrProtocol {
			t.Fatal("diagnostic detail granted command authority", err)
		}
	}
}
