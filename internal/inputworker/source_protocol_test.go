package inputworker

import (
	"bytes"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
)

func TestSourcePacketsRejectIdleKeysAndMalformedAuthority(t *testing.T) {
	binding := Binding{1}
	packets := []sourcePacket{
		{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: binding},
		{kind: sourceReply, op: sourceAdmit, sequence: 1, binding: binding, reply: 1, decision: peer.BoundaryAdmissionBlockedMoveSize},
		{kind: sourceBoundary, sequence: 1, binding: binding, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, x: 300},
		{kind: sourceAction, sequence: 1, binding: binding, reply: 2, action: control.InputAction{Kind: control.InputKeyDown, Code: 0x1e}},
		{kind: sourceReturn, sequence: 1, binding: binding, reply: 2},
		{kind: sourceRetired, sequence: 1, binding: binding, status: 1},
		{kind: sourceCommand, op: sourceCursorShow, sequence: 1, binding: binding, reply: 7},
	}
	for _, p := range packets {
		data, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readSourcePacket(bytes.NewReader(data[:]))
		if err != nil || got != p {
			t.Fatal("invalid roundtrip", err)
		}
		for _, offset := range []int{0, 56, 63} {
			bad := data
			bad[offset] = 255
			if _, err := readSourcePacket(bytes.NewReader(bad[:])); err == nil {
				t.Fatal("malformed padding/magic accepted")
			}
		}
	}
	for _, p := range []sourcePacket{
		{kind: sourceBoundary, sequence: 1, binding: binding, action: control.InputAction{Kind: control.InputKeyDown, Code: 0x1e}},
		{kind: sourceAction, sequence: 1, binding: binding, action: control.InputAction{Kind: control.InputKeyDown, Code: 0x1e}},
		{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: binding, reply: 1},
		{kind: sourceCommand, op: sourceActivate, binding: binding},
		{kind: sourceCommand, op: sourceActivate, sequence: 1},
		{kind: sourceCommand, op: sourceCursorShow, sequence: 1, binding: binding},
		{kind: sourceCommand, op: sourceCursorRestore, sequence: 1, binding: binding, reply: 1},
	} {
		if _, err := encodeSourcePacket(p); err == nil {
			t.Fatal("idle key, unbound action, unsolicited operation or absent authority accepted")
		}
	}
}

func TestSourceLeaseCannotBecomeInjectionOrPresentation(t *testing.T) {
	now := time.Now()
	lease := testLease()
	lease.Capture = true
	data, err := encodeLease(lease, now)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLease(data, now)
	if err != nil || !decoded.Capture || decoded.Presentation {
		t.Fatal("capture mode lost")
	}
	for _, offset := range []int{38, 424, 428} {
		bad := data
		bad[offset] = 1
		if _, err := decodeLease(bad, now); err == nil {
			t.Fatal("capture selector/coordinates accepted")
		}
	}
	lease.Presentation = true
	if _, err := encodeLease(lease, now); err == nil {
		t.Fatal("combined capture/presentation accepted")
	}
}

func TestSourceTimingOnlySurvivesExactActivationReplies(t *testing.T) {
	for _, status := range []byte{0, 1} {
		p := sourcePacket{kind: sourceReply, op: sourceActivate, status: status, sequence: 3, reply: 2, binding: Binding{1}, timing: [3]byte{0, 3, 4}}
		wire, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readSourcePacket(bytes.NewReader(wire[:]))
		if err != nil || got != p {
			t.Fatal("same reply lost outcome or observed phases", got, err)
		}
		unmeasured := wire
		unmeasured[57] = 1
		if _, err := readSourcePacket(bytes.NewReader(unmeasured[:])); err != ErrProtocol {
			t.Fatal("unmeasured authority phase accepted", err)
		}
		for _, offset := range []int{57, 58, 59, 60, 63} {
			bad := wire
			bad[offset] = 255
			if _, err := readSourcePacket(bytes.NewReader(bad[:])); err != ErrProtocol {
				t.Fatal("invalid bucket/reserved byte accepted", offset, err)
			}
		}
		for op := sourceStart; op <= sourceCursorRestore; op++ {
			if op == sourceActivate || op == sourceHeartbeat {
				continue
			}
			bad := wire
			bad[5] = op
			if _, err := readSourcePacket(bytes.NewReader(bad[:])); err != ErrProtocol {
				t.Fatal("activation timing escaped its fixed operation", op, err)
			}
		}
	}
}

func TestSourceHeartbeatTimingRoundTripAndBounds(t *testing.T) {
	p := sourcePacket{kind: sourceReply, op: sourceHeartbeat, sequence: 3, reply: 2, binding: Binding{1}, timing: [3]byte{4, 2, 1}}
	wire, err := encodeSourcePacket(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readSourcePacket(bytes.NewReader(wire[:]))
	if err != nil || got != p {
		t.Fatal("heartbeat phases lost on wire", got, err)
	}
	for _, offset := range []int{57, 58, 59} {
		bad := wire
		bad[offset] = 6
		if _, err := readSourcePacket(bytes.NewReader(bad[:])); err != ErrProtocol {
			t.Fatal("unbounded heartbeat phase accepted", offset, err)
		}
	}
}
