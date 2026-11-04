package inputworker

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func encodeSourceActivationIPC(p sourcePacket, gate, write uint64) ([]byte, error) {
	header, err := encodeSourcePacket(p)
	if err != nil {
		return nil, err
	}
	return appendSourceActivationIPC(header, p.captureIPC.clocks, gate, write), nil
}

func TestSourceActivationIPCOptionalSuffixAndConsecutiveLegacyFrame(t *testing.T) {
	p := sourcePacket{kind: sourceReply, op: sourceActivate, binding: Binding{1}, sequence: 7, reply: 3, captureIPC: &sourceActivationTrace{clocks: sourceActivationClocks{10, 20, 30, 40}}}
	wire, err := encodeSourceActivationIPC(p, 50, 60)
	if err != nil || len(wire) != sourcePacketSize+sourceActivationIPCSize {
		t.Fatal(err, len(wire))
	}
	next := sourcePacket{kind: sourceReply, op: sourceHeartbeat, binding: p.binding, sequence: 8, reply: 4}
	legacy, _ := encodeSourcePacket(next)
	r := bytes.NewReader(append(append([]byte{}, wire...), legacy[:]...))
	got, err := readSourcePacket(r)
	if err != nil || (got.captureIPC == nil || got.captureIPC.clocks != (sourceActivationClocks{10, 20, 30, 40, 50, 60})) || got.reply != p.reply {
		t.Fatal(got, err)
	}
	if got, err := readSourcePacket(r); err != nil || got != next || r.Len() != 0 {
		t.Fatal("suffix consumed another frame", got, err)
	}
	for n := sourcePacketSize; n < len(wire); n++ {
		if _, err := readSourcePacket(bytes.NewReader(wire[:n])); err != io.EOF && err != io.ErrUnexpectedEOF {
			t.Fatal("truncated suffix accepted", n, err)
		}
	}
	for _, clocks := range []sourceActivationClocks{{}, {10, 0, 30, 40}, {10, 20, 15, 40}} {
		p.captureIPC = &sourceActivationTrace{clocks: clocks}
		encoded, err := encodeSourceActivationIPC(p, 50, 60)
		if err != nil || len(encoded) != sourcePacketSize {
			t.Fatal("clock failure changed command", err, len(encoded))
		}
	}
	for i := 0; i < 6; i++ {
		bad := append([]byte{}, wire...)
		binary.BigEndian.PutUint64(bad[sourcePacketSize+i*8:], 0)
		if got, err := readSourcePacket(bytes.NewReader(bad)); err != nil || got.status != 0 || got.captureIPC != nil {
			t.Fatal("bad diagnostics rejected valid command", got, err)
		}
	}
}
