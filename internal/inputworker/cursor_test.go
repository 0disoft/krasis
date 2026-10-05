package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type cursorACKWriter struct {
	bytes.Buffer
	writes int
	short  bool
}

func (w *cursorACKWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.short {
		data = data[:len(data)-1]
	}
	return w.Buffer.Write(data)
}

func TestCursorSnapshotACKUsesOneWriteAndRejectsPartialSuffix(t *testing.T) {
	for _, op := range []operation{opCursorRead, opMoveRead} {
		writer := &cursorACKWriter{}
		ack := response{sequence: 1, op: op, x: -3, y: 4}
		if err := writeResponse(writer, ack); err != nil || writer.writes != 1 || writer.Len() != responseSize+8 {
			t.Fatal("cursor ACK added a pipe write or lost its suffix", err)
		}
		decoded, err := readResponse(writer)
		if err != nil || decoded.op != op || decoded.x != -3 || decoded.y != 4 {
			t.Fatal("cursor snapshot ACK lost framing", err)
		}
		writer = &cursorACKWriter{short: true}
		if !errors.Is(writeResponse(writer, ack), io.ErrShortWrite) || writer.writes != 1 {
			t.Fatal("short cursor ACK write accepted or retried")
		}
		if _, err := readResponse(writer); !errors.Is(err, ErrProtocol) {
			t.Fatal("partial cursor suffix accepted")
		}
		writer = &cursorACKWriter{}
		ack.status, ack.certainty = 1, 1
		if err := writeResponse(writer, ack); err != nil || writer.Len() != responseSize {
			t.Fatal("failed cursor ACK published a snapshot", err)
		}
	}
}

type cursorInput struct {
	recordingInput
	x, y    int32
	sets    int
	readErr error
}

func (p *cursorInput) CursorPosition(context.Context) (int32, int32, error) {
	return p.x, p.y, p.readErr
}
func (p *cursorInput) Move(_ context.Context, x, y int32) error { p.x += x; p.y += y; return nil }
func (p *cursorInput) SetCursorPosition(_ context.Context, x, y int32) error {
	p.x, p.y = x, y
	p.sets++
	return nil
}

func TestCursorWorkerRoundtripSharesAuthoritySequenceAndPressedRecovery(t *testing.T) {
	server, conn := net.Pipe()
	input := &cursorInput{x: -30, y: 20}
	lease := testLease()
	done := make(chan finish, 1)
	go func() {
		r, err := serve(context.Background(), server, input, lease, testExecute, testExecute, time.Now)
		done <- finish{r, err}
	}()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	x, y, err := client.CursorPosition(ctx)
	if err != nil || x != -30 || y != 20 {
		t.Fatal("signed cursor snapshot lost", err)
	}
	if err := client.SetCursorPosition(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	x, y, err = client.CursorPosition(ctx)
	if err != nil || x != 0 || y != 0 {
		t.Fatal("zero cursor target rejected", err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	if err := client.Press(ctx, key); err != nil {
		t.Fatal(err)
	}
	err = client.SetCursorPosition(ctx, 10, 20)
	if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied {
		t.Fatal("cursor set while pressed was not refused")
	}
	_ = client.Close()
	result := await(t, done)
	if result.err == nil || input.sets != 1 || result.report.Recovery.Attempted != 1 {
		t.Fatal("cursor operation bypassed pressed recovery")
	}
}

func TestCursorReadSuffixDoesNotChangeOtherACKsAndTruncationFailsClosed(t *testing.T) {
	var wire bytes.Buffer
	if err := writeResponse(&wire, response{sequence: 1, op: opCursorRead, x: -100, y: 200}); err != nil {
		t.Fatal(err)
	}
	if wire.Len() != responseSize+8 {
		t.Fatal("cursor suffix not bounded")
	}
	if err := writeResponse(&wire, response{sequence: 2, op: opMove}); err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), wire.Bytes()...)
	ack, err := readResponse(&wire)
	if err != nil || ack.x != -100 || ack.y != 200 {
		t.Fatal("cursor suffix framing failed")
	}
	ack, err = readResponse(&wire)
	if err != nil || ack.op != opMove || wire.Len() != 0 {
		t.Fatal("cursor suffix consumed next ACK")
	}
	if _, err := readResponse(bytes.NewReader(data[:responseSize+7])); !errors.Is(err, ErrProtocol) {
		t.Fatal("truncated cursor snapshot accepted")
	}
	for _, op := range []operation{opCursorRead, opCursorSet} {
		raw, err := encodeRequest(request{op: op, sequence: 1, binding: Binding{1}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readRequest(bytes.NewReader(raw[:])); err != nil {
			t.Fatal(err)
		}
		raw[6] = 1
		if _, err := readRequest(bytes.NewReader(raw[:])); !errors.Is(err, ErrProtocol) {
			t.Fatal("cursor command accepted token selector")
		}
	}
}

func TestCursorWrongBindingAndLostResponseNeverReplayOrFallback(t *testing.T) {
	for _, lost := range []bool{false, true} {
		server, conn := net.Pipe()
		lease := testLease()
		client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		if lost {
			go func() { defer close(done); defer server.Close(); _, _ = readRequest(server) }()
			err = client.SetCursorPosition(context.Background(), 1, 2)
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown {
				t.Fatal("lost cursor acknowledgement fabricated certainty")
			}
			if err := client.SetCursorPosition(context.Background(), 1, 2); err == nil {
				t.Fatal("uncertain cursor command replayed")
			}
		} else {
			input := &cursorInput{}
			client.lease.Binding = Binding{2}
			go func() {
				defer close(done)
				_, _ = serve(context.Background(), server, input, lease, testExecute, testExecute, time.Now)
			}()
			_, _, err = client.CursorPosition(context.Background())
			if err == nil {
				t.Fatal("wrong-binding cursor query accepted")
			}
		}
		_ = client.Close()
		<-done
	}
}

func TestMotionSnapshotUsesOneOperationAndReadFailureStaysOutcomeUnknown(t *testing.T) {
	for _, failed := range []bool{false, true} {
		server, conn := net.Pipe()
		input := &cursorInput{x: 10, y: 20}
		lease := testLease()
		if failed {
			input.readErr = notSent(ErrAuthority)
		}
		done := make(chan finish, 1)
		scopes := 0
		do := func(ctx context.Context, f func() error) error { scopes++; return testExecute(ctx, f) }
		go func() {
			r, err := serve(context.Background(), server, input, lease, do, testExecute, time.Now)
			done <- finish{r, err}
		}()
		client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
		if err != nil {
			t.Fatal(err)
		}
		x, y, err := client.MoveWithPosition(context.Background(), 1, 2)
		if failed {
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown {
				t.Fatal("applied motion followed by failed snapshot became unapplied")
			}
		} else if err != nil || x != 11 || y != 22 {
			t.Fatal("motion snapshot lost", err)
		}
		_ = client.Close()
		result := await(t, done)
		if scopes != 1 || input.x != 11 || input.y != 22 || (result.err != nil) != failed {
			t.Fatal("motion snapshot repeated operation or hid failure")
		}
	}
}
