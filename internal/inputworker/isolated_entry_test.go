package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type entryWriteFunc func([]byte) (int, error)

func (f entryWriteFunc) Write(data []byte) (int, error) { return f(data) }

type entryTransport struct {
	handle   func(io.Reader, io.Writer) error
	commands []operation
	drop     bool
}

func (p *entryTransport) Close() error { return nil }
func (p *entryTransport) ExchangeInput(_ context.Context, exchange func(io.Writer, io.Reader) error) error {
	var reply bytes.Buffer
	return exchange(entryWriteFunc(func(data []byte) (int, error) {
		command, err := readRequest(bytes.NewReader(data))
		if err != nil {
			return 0, err
		}
		if command.op != opHeartbeat {
			p.commands = append(p.commands, command.op)
		}
		if err := p.handle(bytes.NewReader(data), &reply); err != nil {
			return 0, err
		}
		if p.drop && command.op != opHeartbeat {
			reply.Reset()
		}
		return len(data), nil
	}), &reply)
}

func TestIsolatedEntryUsesOneNativeAdmissionAndPreservesProbe(t *testing.T) {
	for _, op := range []operation{opCursorEntry, opCursorEntryProbe} {
		t.Run(map[operation]string{opCursorEntry: "entry", opCursorEntryProbe: "probe"}[op], func(t *testing.T) {
			native := &probingInput{placingInput: placingInput{cursorInput: cursorInput{x: -400, y: 600}}}
			admissions := 0
			transport := &entryTransport{handle: PortableInputHandler(native, func(_ context.Context, work func() error) error { admissions++; return work() })}
			client, err := newPortableClient(context.Background(), transport)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			admissions = 0
			var pressed control.PressedState
			var ack response
			err = applyCommand(context.Background(), &isolatedInput{Client: client}, &pressed, request{op: op, applyDetail: true}, &ack)
			if err != nil || len(transport.commands) != 1 || transport.commands[0] != op || admissions != 1 {
				t.Fatal("entry split into multiple native transactions", err, transport.commands, admissions)
			}
			if ack.beforeX != -400 || ack.beforeY != 600 || ack.targetX != -3 || ack.x != -2 || ack.y != 4 || ack.applyDetail == nil || !ack.applyDetail.Valid() {
				t.Fatal("entry snapshot, target, readback or profile changed", ack)
			}
			if (op == opCursorEntryProbe) != (native.probes == 1 && ack.observation != nil) {
				t.Fatal("probe request/observation lost", native.probes, ack.observation)
			}
		})
	}
}

func TestIsolatedEntryKeepsPressedGuardAndFailureCertainty(t *testing.T) {
	for _, mode := range []string{"pressed", "snapshot_failed", "placement_failed", "lost_ack"} {
		t.Run(mode, func(t *testing.T) {
			native := &placingInput{cursorInput: cursorInput{x: 1, y: 2}}
			if mode == "snapshot_failed" {
				native.readErr = errors.New("snapshot failed")
			}
			if mode == "placement_failed" {
				native.placeErr = unknownInput(ErrProtocol)
			}
			transport := &entryTransport{handle: PortableInputHandler(native, func(_ context.Context, work func() error) error { return work() }), drop: mode == "lost_ack"}
			client, err := newPortableClient(context.Background(), transport)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			var pressed control.PressedState
			if mode == "pressed" {
				_, _ = pressed.RecordPress(control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65})
			}
			var ack response
			err = applyCommand(context.Background(), &isolatedInput{Client: client}, &pressed, request{op: opCursorEntry, x: 4, y: 5}, &ack)
			want := application.InjectionNotApplied
			if mode == "placement_failed" || mode == "lost_ack" {
				want = application.InjectionOutcomeUnknown
			}
			if certainty, known := application.InjectionCertaintyOf(err); !known || certainty != want || ack.beforeX != 0 || ack.beforeY != 0 {
				t.Fatal("failure changed certainty or exposed rollback snapshot", err, ack)
			}
			if mode == "pressed" || mode == "snapshot_failed" {
				if native.places != 0 {
					t.Fatal("failed precondition moved cursor")
				}
			}
			if mode == "lost_ack" {
				before := len(transport.commands)
				if err := applyCommand(context.Background(), &isolatedInput{Client: client}, &pressed, request{op: opCursorEntry, x: 4, y: 5}, &ack); err == nil || len(transport.commands) != before || native.places != 1 {
					t.Fatal("lost entry ACK replayed cursor placement", err, transport.commands, native.places)
				}
			}
		})
	}
}
