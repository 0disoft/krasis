package inputworker

import (
	"bytes"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceStartupNeverMistakesEarlyEventOrUnconfirmedRetirementForReady(t *testing.T) {
	binding := Binding{1}
	for _, p := range []sourcePacket{
		{kind: sourceRetired, sequence: 1, binding: binding},
		{kind: sourceRetired, sequence: 1, binding: binding, status: 1},
		{kind: sourceRetired, sequence: 2, binding: binding},
		{kind: sourceRetired, sequence: 1, binding: Binding{2}},
		{kind: sourceBoundary, sequence: 1, binding: binding, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}},
	} {
		data, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		profile, retired, err := readSourceStartup(bytes.NewReader(data[:]), binding)
		want := p.kind == sourceRetired && p.sequence == 1 && p.status == 0 && p.binding == binding
		if (err == nil) != want || retired != want || profile != nil {
			t.Fatal("startup claim incorrectly accepted", p.kind, err)
		}
	}
}
