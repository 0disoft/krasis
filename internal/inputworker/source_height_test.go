package inputworker

import (
	"bytes"
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestSourceHeightBoundToBoundaryOrDeactivate(t *testing.T) {
	for _, h := range []control.EdgeHeight{0, 1, 32769, 65536, 65537} {
		for _, kind := range []byte{sourceBoundary, sourceCommand, sourceReply, sourceAction} {
			p := sourcePacket{kind: kind, sequence: 1, binding: Binding{1}, height: h}
			switch kind {
			case sourceBoundary:
				p.action = control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
				p.x = 1919
			case sourceCommand:
				p.op = sourceDeactivate
			case sourceReply:
				p.op = sourceDeactivate
				p.reply = 1
			case sourceAction:
				p.reply = 1
				p.action = control.InputAction{Kind: control.InputKeyDown, Code: 30}
			}
			data, err := encodeSourcePacket(p)
			valid := h.Valid() && (h == 0 || kind == sourceBoundary || kind == sourceCommand)
			if !valid {
				if err == nil {
					t.Fatal("height escaped presentation boundary", p)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := readSourcePacket(bytes.NewReader(data[:]))
			if err != nil || got != p {
				t.Fatal("private stream lost cursor height", got, err)
			}
		}
	}
}
