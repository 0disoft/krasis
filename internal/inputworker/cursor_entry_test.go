package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

func TestCursorEntryACKKeepsSnapshotPlacementAndNextResponseAligned(t *testing.T) {
	for _, op := range []operation{opCursorEntry, opCursorEntryProbe} {
		for _, profiled := range []bool{false, true} {
			w := &cursorACKWriter{}
			ack := response{op: op, sequence: 1, beforeX: -400, beforeY: 600, x: -2, y: 4, targetX: -3, targetY: 4}
			if profiled {
				ack.applyDetail = &handoff.WorkerApplyProfile{Micros: [2]uint32{1, 0}, Measured: 1, SubmissionMicros: 1, SubmissionCalls: 1}
			}
			if err := writeResponse(w, ack); err != nil || w.writes != 1 {
				t.Fatal("entry response added a write", err)
			}
			original := append([]byte(nil), w.Bytes()...)
			if err := writeResponse(w, response{sequence: 2, op: opMove}); err != nil {
				t.Fatal(err)
			}
			r := iotest.OneByteReader(w)
			got, err := readResponse(r)
			if err != nil || got.beforeX != -400 || got.beforeY != 600 || got.targetX != -3 || got.x != -2 || got.y != 4 || got.targetY != 4 {
				t.Fatal("entry snapshot/target/readback changed", err)
			}
			if next, err := readResponse(r); err != nil || next.sequence != 2 {
				t.Fatal("next response shifted", err)
			}
			if _, err := readResponse(bytes.NewReader(original[:len(original)-1])); !errors.Is(err, ErrProtocol) {
				t.Fatal("partial snapshot was accepted")
			}
			data, err := encodeRequest(request{op: op, sequence: 1})
			if err != nil {
				t.Fatal(err)
			}
			if got, err := readRequest(bytes.NewReader(data[:])); err != nil || got.op != op {
				t.Fatal("zero entry target lost", err)
			}
		}
	}
}

func TestCursorEntryUsesOneScopeAndPreservesFailureAndPressedRecovery(t *testing.T) {
	for _, mode := range []string{"ok", "snapshot_failed", "placement_failed", "pressed"} {
		t.Run(mode, func(t *testing.T) {
			server, conn := net.Pipe()
			input := &placingInput{cursorInput: cursorInput{x: -300, y: 200}}
			if mode == "snapshot_failed" {
				input.readErr = errors.New("snapshot failed")
			}
			if mode == "placement_failed" {
				input.placeErr = unknownInput(ErrProtocol)
			}
			done := make(chan finish, 1)
			scopes := 0
			execute := func(ctx context.Context, f func() error) error { scopes++; return testExecute(ctx, f) }
			go func() {
				r, err := serve(context.Background(), server, input, testLease(), execute, testExecute, time.Now)
				done <- finish{r, err}
			}()
			client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if mode == "pressed" {
				if err := client.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}); err != nil {
					t.Fatal(err)
				}
			}
			entry, err := client.EnterCursor(context.Background(), 0, 0)
			if mode == "ok" {
				if err != nil || entry.BeforeX != -300 || entry.BeforeY != 200 || entry.TargetX != -3 || entry.X != -2 || scopes != 1 || input.places != 1 {
					t.Fatal("entry split or receipt fabricated", err)
				}
			} else {
				want := application.InjectionNotApplied
				if mode == "placement_failed" {
					want = application.InjectionOutcomeUnknown
				}
				if c, known := application.InjectionCertaintyOf(err); !known || c != want || entry != (application.CursorEntry{}) {
					t.Fatal("failed entry published a receipt or changed certainty", err)
				}
			}
			_ = client.Close()
			result := await(t, done)
			if (mode == "pressed" || mode == "snapshot_failed") && input.places != 0 {
				t.Fatal("entry wrote after failed precondition")
			}
			if mode == "pressed" && result.report.Recovery.Attempted != 1 {
				t.Fatal("entry bypassed pressed recovery")
			}
		})
	}
}

func TestCursorEntryLostACKDoesNotReplayOrExposeSnapshot(t *testing.T) {
	server, conn := net.Pipe()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); _, _ = readRequest(server) }()
	entry, err := client.EnterCursor(context.Background(), 1, 2)
	if c, known := application.InjectionCertaintyOf(err); !known || c != application.InjectionOutcomeUnknown || entry != (application.CursorEntry{}) {
		t.Fatal("lost receipt claimed a snapshot or certainty", err)
	}
	if _, err := client.EnterCursor(context.Background(), 1, 2); err == nil {
		t.Fatal("uncertain entry was replayed")
	}
	<-done
}
