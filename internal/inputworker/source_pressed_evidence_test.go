package inputworker

import (
	"bytes"
	"testing"

	"github.com/0disoft/krasis/internal/peer"
)

func TestSourcePressedEvidenceRemainsBoundToAdmissionReply(t *testing.T) {
	for evidence := byte(1); evidence <= 3; evidence++ {
		p := sourcePacket{kind: sourceReply, op: sourceAdmit, sequence: 1, binding: Binding{1}, reply: 2, decision: peer.BoundaryAdmissionBlockedPressedInput, pressedEvidence: evidence}
		data, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readSourcePacket(bytes.NewReader(data[:]))
		if err != nil || got != p || sourcePressedEvidence(sourcePressedCode(got.pressedEvidence)) != evidence {
			t.Fatal(got, err)
		}
		for _, mutate := range []func(*sourcePacket){
			func(p *sourcePacket) { p.pressedEvidence = 4 },
			func(p *sourcePacket) { p.op = sourceHeartbeat; p.decision = 0 },
			func(p *sourcePacket) { p.decision = peer.BoundaryAdmissionAllowed },
			func(p *sourcePacket) { p.status = 1 },
			func(p *sourcePacket) { p.kind = sourceCommand; p.reply = 0; p.decision = 0 },
		} {
			bad := p
			mutate(&bad)
			if bad.valid() {
				t.Fatal("evidence accepted outside a successful pressed admission reply", bad)
			}
		}
	}
}
