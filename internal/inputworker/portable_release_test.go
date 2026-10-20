package inputworker

import (
	"bytes"
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestReleaseTransportRejectsAllNonReleaseNativeCommands(t *testing.T) {
	for _, op := range []operation{opPress, opMove, opWheel, opCursorRead, opCursorSet, opMoveRead, opCursorPlace, opCursorPlaceProbe, opClose} {
		input := &recordingInput{}
		checks := 0
		handler := PortableReleaseHandler(input, func(ctx context.Context, work func() error) error { checks++; return work() })
		p := request{op: op, sequence: 1, binding: Binding{1}, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, x: 1}
		data, err := encodeRequest(p)
		if err != nil { // Encode only legal shapes; malformed commands are denied too.
			p.token, p.x = control.PressedToken{}, 0
			if op == opMove || op == opWheel || op == opMoveRead {
				p.x = 1
			}
			if op == opPress {
				p.token = control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			}
			data, err = encodeRequest(p)
		}
		if err != nil {
			t.Fatal(op, err)
		}
		var response bytes.Buffer
		if err := handler(bytes.NewReader(data[:]), &response); err != ErrProtocol || checks != 0 || input.pressed != 0 || input.released != 0 || response.Len() != 0 {
			t.Fatal("release transport admitted another capability", op, err, checks)
		}
	}
}

func TestReleaseTransportKeepsFreshChecksAndOrderedBinding(t *testing.T) {
	input := &recordingInput{}
	checks := 0
	handler := PortableReleaseHandler(input, func(ctx context.Context, work func() error) error { checks++; return work() })
	for i, op := range []operation{opHeartbeat, opRelease} {
		p := request{op: op, sequence: uint64(i + 1), binding: Binding{1}}
		if op == opRelease {
			p.token = control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
		}
		data, err := encodeRequest(p)
		if err != nil {
			t.Fatal(err)
		}
		var wire bytes.Buffer
		if err := handler(bytes.NewReader(data[:]), &wire); err != nil {
			t.Fatal(err)
		}
		ack, err := readResponse(&wire)
		if err != nil || ack.status != 0 || ack.sequence != p.sequence {
			t.Fatal(ack, err)
		}
	}
	if checks != 2 || input.released != 1 || input.pressed != 0 {
		t.Fatal(checks, input)
	}
	data, _ := encodeRequest(request{op: opHeartbeat, sequence: 3, binding: Binding{2}})
	if err := handler(bytes.NewReader(data[:]), &bytes.Buffer{}); err != ErrProtocol || checks != 2 {
		t.Fatal("foreign binding passed", err)
	}
}
