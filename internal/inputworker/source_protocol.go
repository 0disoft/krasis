package inputworker

import (
	"encoding/binary"
	"io"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/peer"
)

const sourcePacketSize = 64
const (
	sourceCommand byte = iota + 1
	sourceReply
	sourceBoundary
	sourceAction
	sourceReturn
	sourceRetired
)
const (
	sourceStart byte = iota + 1
	sourceAdmit
	sourceActivate
	sourceDeactivate
	sourceHeartbeat
	sourceStop
	sourceCursorPrepare
	sourceCursorShow
	sourceCursorRestore
)

// Private, authenticated local stream only. Idle observations carry pointer
// motion, never PIN keys. Actions require the exact active capture generation.
type sourcePacket struct {
	kind, op, status byte
	sequence         uint64
	binding          Binding
	reply            uint64
	action           control.InputAction
	x                int32
	decision         peer.BoundaryAdmissionDecision
	pressedEvidence  byte
	timing           [3]byte               // Activation or heartbeat reply; fixed operation-specific buckets.
	capture          handoff.CaptureNative // Only an activation reply; never input or authority.
}

func (p sourcePacket) valid() bool {
	if p.sequence == 0 || p.binding == (Binding{}) || p.status > 1 {
		return false
	}
	if !p.capture.Valid() || p.capture != (handoff.CaptureNative{}) && (p.kind != sourceReply || p.op != sourceActivate) {
		return false
	}
	if p.pressedEvidence != 0 && (p.pressedEvidence > 3 || p.kind != sourceReply || p.op != sourceAdmit || p.status != 0 || p.decision != peer.BoundaryAdmissionBlockedPressedInput) {
		return false
	}
	if p.timing != ([3]byte{}) && (p.kind != sourceReply || p.op != sourceActivate && p.op != sourceHeartbeat) {
		return false
	}
	if p.op != sourceHeartbeat && p.timing[0] != 0 { // Activation has no reader measurement.
		return false
	}
	for _, bucket := range p.timing {
		if bucket > 5 {
			return false
		}
	}
	switch p.kind {
	case sourceCommand:
		return p.op >= sourceStart && p.op <= sourceCursorRestore && p.status == 0 && ((p.op == sourceCursorShow && p.reply != 0) || (p.op != sourceCursorShow && p.reply == 0)) && p.action == (control.InputAction{}) && p.x == 0 && p.decision == 0
	case sourceReply:
		return p.op >= sourceStart && p.op <= sourceCursorRestore && p.reply != 0 && p.action == (control.InputAction{}) && p.x == 0 && (p.op == sourceAdmit && p.decision.Valid() || p.decision == 0)
	case sourceBoundary:
		return p.op == 0 && p.status == 0 && p.reply == 0 && p.decision == 0 && p.action.Kind == control.InputPointerMove && p.action.Validate() == nil
	case sourceAction:
		return p.op == 0 && p.status == 0 && p.reply != 0 && p.x == 0 && p.decision == 0 && p.action.Validate() == nil
	case sourceReturn:
		return p.op == 0 && p.status == 0 && p.reply != 0 && p.action == (control.InputAction{}) && p.x == 0 && p.decision == 0
	case sourceRetired:
		return (p.op == 0 || p.op == 1 && p.status == 0) && p.reply == 0 && p.action == (control.InputAction{}) && p.x == 0 && p.decision == 0
	}
	return false
}

func encodeSourcePacket(p sourcePacket) ([sourcePacketSize]byte, error) {
	var data [sourcePacketSize]byte
	if !p.valid() {
		return data, ErrProtocol
	}
	copy(data[:4], "KSC1")
	data[4], data[5], data[6] = p.kind, p.op, p.status
	binary.BigEndian.PutUint64(data[8:16], p.sequence)
	copy(data[16:32], p.binding[:])
	binary.BigEndian.PutUint64(data[32:40], p.reply)
	for i, kind := range []control.InputActionKind{control.InputKeyDown, control.InputKeyUp, control.InputButtonDown, control.InputButtonUp, control.InputPointerMove, control.InputWheel} {
		if p.action.Kind == kind {
			data[7] = byte(i + 1)
		}
	}
	code := p.action.Code
	if p.kind == sourceReply {
		code = uint32(p.decision)
	}
	binary.BigEndian.PutUint32(data[40:44], code)
	binary.BigEndian.PutUint32(data[44:48], uint32(p.action.DeltaX))
	binary.BigEndian.PutUint32(data[48:52], uint32(p.action.DeltaY))
	binary.BigEndian.PutUint32(data[52:56], uint32(p.x))
	data[56] = p.pressedEvidence
	copy(data[57:60], p.timing[:])
	if p.kind == sourceReply && p.op == sourceActivate {
		bits := packSourceCapture(p.capture)
		binary.BigEndian.PutUint32(data[52:56], uint32(bits>>32))
		binary.BigEndian.PutUint32(data[60:64], uint32(bits))
	}
	return data, nil
}

func readSourcePacket(r io.Reader) (sourcePacket, error) {
	var data [sourcePacketSize]byte
	if _, err := io.ReadFull(r, data[:]); err != nil {
		return sourcePacket{}, err
	}
	if string(data[:4]) != "KSC1" || data[7] > 6 {
		return sourcePacket{}, ErrProtocol
	}
	p := sourcePacket{kind: data[4], op: data[5], status: data[6], sequence: binary.BigEndian.Uint64(data[8:16]), reply: binary.BigEndian.Uint64(data[32:40]), x: int32(binary.BigEndian.Uint32(data[52:56]))}
	if p.kind == sourceReply && p.op == sourceActivate {
		bits := uint64(binary.BigEndian.Uint32(data[52:56]))<<32 | uint64(binary.BigEndian.Uint32(data[60:64]))
		var ok bool
		p.capture, ok = unpackSourceCapture(bits)
		if !ok {
			return sourcePacket{}, ErrProtocol
		}
		p.x = 0
	} else if !zeroBytes(data[60:]) {
		return sourcePacket{}, ErrProtocol
	}
	p.pressedEvidence = data[56]
	copy(p.timing[:], data[57:60])
	copy(p.binding[:], data[16:32])
	if data[7] != 0 {
		p.action.Kind = []control.InputActionKind{control.InputKeyDown, control.InputKeyUp, control.InputButtonDown, control.InputButtonUp, control.InputPointerMove, control.InputWheel}[data[7]-1]
	}
	p.action.Code = binary.BigEndian.Uint32(data[40:44])
	if p.kind == sourceReply {
		p.decision = peer.BoundaryAdmissionDecision(p.action.Code)
		p.action.Code = 0
	}
	p.action.DeltaX, p.action.DeltaY = int32(binary.BigEndian.Uint32(data[44:48])), int32(binary.BigEndian.Uint32(data[48:52]))
	if !p.valid() {
		return sourcePacket{}, ErrProtocol
	}
	return p, nil
}
