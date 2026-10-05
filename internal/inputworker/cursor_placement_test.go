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

type placingInput struct {
	cursorInput
	places   int
	placeErr error
}

func (p *placingInput) PlaceCursor(context.Context, int32, int32) (application.CursorPlacement, error) {
	p.places++
	return application.CursorPlacement{TargetX: -3, TargetY: 4, X: -2, Y: 4}, p.placeErr
}

func TestPlacementACKKeepsTargetAndReadbackSeparateAndFollowingACKAligned(t *testing.T) {
	for _, profile := range []bool{false, true} {
		w := &cursorACKWriter{}
		ack := response{sequence: 1, op: opCursorPlace, x: -2, y: 4, targetX: -3, targetY: 4}
		if profile {
			ack.applyDetail = &handoff.WorkerApplyProfile{}
		}
		if err := writeResponse(w, ack); err != nil || w.writes != 1 {
			t.Fatal("placement ACK write", err)
		}
		original := append([]byte(nil), w.Bytes()...)
		_ = writeResponse(w, response{sequence: 2, op: opMove})
		r := iotest.OneByteReader(w)
		got, err := readResponse(r)
		if err != nil || got.x != -2 || got.targetX != -3 || got.targetY != 4 || got.y != 4 {
			t.Fatal("placement readback fabricated or lost", err)
		}
		if got, err = readResponse(r); err != nil || got.sequence != 2 {
			t.Fatal("next ACK shifted", err)
		}
		if _, err := readResponse(bytes.NewReader(original[:len(original)-1])); !errors.Is(err, ErrProtocol) {
			t.Fatal("partial placement accepted")
		}
	}
}

func TestPlacementWorkerOneAuthorizedOperationAndPressedGuard(t *testing.T) {
	for _, mode := range []string{"ok", "failed", "pressed"} {
		server, conn := net.Pipe()
		input := &placingInput{}
		if mode == "failed" {
			input.placeErr = unknownInput(ErrProtocol)
		}
		lease := testLease()
		done := make(chan finish, 1)
		scopes := 0
		execute := func(ctx context.Context, f func() error) error { scopes++; return testExecute(ctx, f) }
		go func() {
			r, err := serve(context.Background(), server, input, lease, execute, testExecute, time.Now)
			done <- finish{r, err}
		}()
		client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "pressed" {
			if err := client.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}); err != nil {
				t.Fatal(err)
			}
		}
		p, err := client.PlaceCursor(context.Background(), 0, 0)
		if mode == "ok" {
			if err != nil || p.X != -2 || p.TargetX != -3 || input.places != 1 || scopes != 1 {
				t.Fatal("placement split or mismatch hidden", err)
			}
		} else {
			want := application.InjectionOutcomeUnknown
			if mode == "pressed" {
				want = application.InjectionNotApplied
			}
			if c, known := application.InjectionCertaintyOf(err); !known || c != want {
				t.Fatal("placement certainty changed", err)
			}
		}
		_ = client.Close()
		result := await(t, done)
		if mode == "pressed" && (input.places != 0 || result.report.Recovery.Attempted != 1) {
			t.Fatal("placement bypassed pressed recovery")
		}
	}
}

func TestLostPlacementACKNeverReplays(t *testing.T) {
	server, conn := net.Pipe()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); _, _ = readRequest(server) }()
	_, err = client.PlaceCursor(context.Background(), 1, 2)
	if c, known := application.InjectionCertaintyOf(err); !known || c != application.InjectionOutcomeUnknown {
		t.Fatal("lost receipt fabricated certainty", err)
	}
	if _, err := client.PlaceCursor(context.Background(), 1, 2); err == nil {
		t.Fatal("uncertain placement replayed")
	}
	_ = client.Close()
	<-done
}
