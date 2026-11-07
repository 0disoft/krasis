package inputworker

import (
	"bytes"
	"encoding/binary"
	"github.com/0disoft/krasis/internal/handoff"
	"io"
	"testing"
)

func TestSourceActivationAuthoritySuffixIsOptionalBoundedAndFrameAligned(t *testing.T) {
	p := sourcePacket{kind: sourceReply, op: sourceActivate, binding: Binding{1}, sequence: 7, reply: 3}
	header, _ := encodeSourcePacket(p)
	clocks := sourceActivationClocks{10, 20, 30, 40}
	for _, detail := range []handoff.CaptureAuthorityProfile{
		{Measured: true}, {Measured: true, WaitMicros: 17, NativeMicros: 3}, {Measured: true, WaitMicros: uint32(handoff.Window.Microseconds())},
	} {
		wire := appendSourceActivationIPC(header, clocks, 50, 60, detail)
		if len(wire) != sourcePacketSize+sourceActivationIPCSize+sourceActivationAuthoritySize || wire[56] != sourceActivationAuthorityFlag {
			t.Fatal("missing new extension")
		}
		next, _ := encodeSourcePacket(sourcePacket{kind: sourceReply, op: sourceHeartbeat, binding: p.binding, sequence: 8, reply: 4})
		r := bytes.NewReader(append(append([]byte{}, wire...), next[:]...))
		got, err := readSourcePacket(r)
		if err != nil || got.captureIPC == nil || got.captureIPC.readAuthority != detail || got.status != 0 || got.reply != p.reply {
			t.Fatal("detail changed reply", got, err)
		}
		if got, err := readSourcePacket(r); err != nil || got.op != sourceHeartbeat || r.Len() != 0 {
			t.Fatal("extension consumed next frame", got, err)
		}
		for n := sourcePacketSize; n < len(wire); n++ {
			if _, err := readSourcePacket(bytes.NewReader(wire[:n])); err != io.EOF && err != io.ErrUnexpectedEOF {
				t.Fatal("truncated detail accepted", n, err)
			}
		}
		for _, encoded := range []uint32{0, ^uint32(0)} {
			bad := append([]byte{}, wire...)
			binary.BigEndian.PutUint32(bad[sourcePacketSize+sourceActivationIPCSize:], encoded)
			got, err := readSourcePacket(bytes.NewReader(bad))
			if err != nil || got.status != 0 || got.captureIPC == nil || got.captureIPC.readAuthority.Measured {
				t.Fatal("invalid diagnostic changed command outcome", got, err)
			}
		}
		bad := append([]byte{}, wire...)
		binary.BigEndian.PutUint64(bad[sourcePacketSize:], 0)
		r = bytes.NewReader(append(bad, next[:]...))
		if got, err := readSourcePacket(r); err != nil || got.captureIPC != nil {
			t.Fatal("invalid clocks retained details", got, err)
		}
		if _, err := readSourcePacket(r); err != nil || r.Len() != 0 {
			t.Fatal("discarded diagnostics misaligned next frame", err)
		}
	}
	for _, detail := range []handoff.CaptureAuthorityProfile{{}, {WaitMicros: 1}, {Measured: true, WaitMicros: ^uint32(0)}} {
		wire := appendSourceActivationIPC(header, clocks, 50, 60, detail)
		if len(wire) != sourcePacketSize+sourceActivationIPCSize || wire[56] != sourceActivationIPCFlag {
			t.Fatal("invalid optional data changed legacy layout")
		}
	}
	for _, mutate := range []func([]byte){
		func(w []byte) { w[56] = 0x82 },
		func(w []byte) { w[5] = sourceHeartbeat },
	} {
		wire := appendSourceActivationIPC(header, clocks, 50, 60, handoff.CaptureAuthorityProfile{Measured: true})
		mutate(wire)
		if _, err := readSourcePacket(bytes.NewReader(wire)); err != ErrProtocol {
			t.Fatal("reserved/other-command flag was accepted", err)
		}
	}
}
