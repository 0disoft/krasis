package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type recordingInput struct {
	mu                   sync.Mutex
	pressed, released    int
	pressErr, releaseErr error
}

func (p *recordingInput) Press(context.Context, control.PressedToken) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pressed++
	return p.pressErr
}
func (p *recordingInput) Release(context.Context, control.PressedToken) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.released++
	return p.releaseErr
}
func (*recordingInput) Move(context.Context, int32, int32) error  { return nil }
func (*recordingInput) Wheel(context.Context, int32, int32) error { return nil }

func testLease() Lease {
	return Lease{Identity: application.LeaseIdentity{SessionID: "test-session", LeaseID: "test-lease", Epoch: "test-epoch"}, Binding: Binding{1}, ExpiresAt: time.Now().Add(time.Minute)}
}

func testExecute(ctx context.Context, work func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return work()
}

type finish struct {
	report Report
	err    error
}

func startWorker(t *testing.T, ctx context.Context, input *recordingInput, lease Lease) (net.Conn, <-chan finish) {
	t.Helper()
	server, client := net.Pipe()
	done := make(chan finish, 1)
	go func() {
		report, err := serve(ctx, server, input, lease, testExecute, testExecute, time.Now)
		done <- finish{report, err}
	}()
	t.Cleanup(func() { _ = client.Close() })
	return client, done
}
func send(t *testing.T, conn net.Conn, command request) [responseSize]byte {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	data, err := encodeRequest(command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(data[:]); err != nil {
		t.Fatal(err)
	}
	var ack [responseSize]byte
	if command.op != opClose {
		if _, err := io.ReadFull(conn, ack[:]); err != nil {
			t.Fatal(err)
		}
	}
	return ack
}
func await(t *testing.T, done <-chan finish) finish {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(time.Second):
		t.Fatal("input worker did not finish")
		return finish{}
	}
}

func TestEOFRecoversHeldInput(t *testing.T) {
	input := &recordingInput{}
	lease := testLease()
	client, done := startWorker(t, context.Background(), input, lease)
	ack := send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}})
	if ack[6] != 0 {
		t.Fatal("press rejected")
	}
	_ = client.Close()
	result := await(t, done)
	if result.err != nil || result.report.Completed != 1 || result.report.Recovery.Released != 1 || input.released != 1 {
		t.Fatalf("EOF recovery: %+v released=%d", result, input.released)
	}
}

func TestCancellationUsesIndependentRecoveryContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := &recordingInput{}
	lease := testLease()
	client, done := startWorker(t, ctx, input, lease)
	send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedMouseButton, Code: 1}})
	cancel()
	result := await(t, done)
	if !errors.Is(result.err, context.Canceled) || result.report.Recovery.Released != 1 {
		t.Fatalf("canceled recovery: %+v", result)
	}
}

func TestDuplicateOrWrongBindingStopsWithoutApplyingAgain(t *testing.T) {
	for _, wrongBinding := range []bool{false, true} {
		t.Run(map[bool]string{false: "sequence", true: "binding"}[wrongBinding], func(t *testing.T) {
			input := &recordingInput{}
			lease := testLease()
			client, done := startWorker(t, context.Background(), input, lease)
			command := request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}}
			send(t, client, command)
			if wrongBinding {
				command.sequence = 2
				command.binding[0]++
			}
			data, _ := encodeRequest(command)
			_, _ = client.Write(data[:])
			result := await(t, done)
			if !errors.Is(result.err, ErrProtocol) || input.pressed != 1 || result.report.Recovery.Released != 1 {
				t.Fatalf("replay recovery: %+v presses=%d", result, input.pressed)
			}
		})
	}
}

func TestUncertainPressRemainsInRecoveryPlan(t *testing.T) {
	input := &recordingInput{pressErr: &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Requested: 1}}
	lease := testLease()
	client, done := startWorker(t, context.Background(), input, lease)
	ack := send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}})
	result := await(t, done)
	if ack[6] != 1 || ack[7] != 3 || result.report.Recovery.Released != 1 || input.pressed != 1 {
		t.Fatalf("uncertain recovery: %+v status=%d certainty=%d", result, ack[6], ack[7])
	}
}

func TestUnappliedPressDoesNotReleaseUnheldKey(t *testing.T) {
	input := &recordingInput{pressErr: &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1}}
	lease := testLease()
	client, done := startWorker(t, context.Background(), input, lease)
	ack := send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}})
	result := await(t, done)
	if ack[7] != 1 || result.report.Recovery.Attempted != 0 || input.released != 0 {
		t.Fatalf("unapplied recovery: %+v", result)
	}
}

func TestLeaseExpiryCannotBeExtendedByHeartbeats(t *testing.T) {
	lease := testLease()
	lease.ExpiresAt = time.Now().Add(80 * time.Millisecond)
	input := &recordingInput{}
	client, done := startWorker(t, context.Background(), input, lease)
	send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}})
	send(t, client, request{op: opHeartbeat, sequence: 2, binding: lease.Binding})
	result := await(t, done)
	if !errors.Is(result.err, ErrExpired) || result.report.Recovery.Released != 1 {
		t.Fatalf("lease expiry: %+v", result)
	}
}

func TestRecoveryFailurePreservesRemainingCountWithoutPayload(t *testing.T) {
	input := &recordingInput{releaseErr: errors.New("synthetic private input")}
	lease := testLease()
	client, done := startWorker(t, context.Background(), input, lease)
	send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}})
	_ = client.Close()
	result := await(t, done)
	if !errors.Is(result.err, ErrRecovery) || result.report.Recovery.Remaining != 1 || strings.Contains(result.err.Error(), "synthetic private input") {
		t.Fatalf("recovery failure: %+v", result)
	}
}

func TestMalformedFramesNeverEchoInputPayload(t *testing.T) {
	lease := testLease()
	base, _ := encodeRequest(request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}})
	for _, offset := range []int{0, 4, 5, 6, 7, 36, 40, 47} {
		bad := base
		bad[offset] = 0xff
		if _, err := readRequest(bytes.NewReader(bad[:])); err != ErrProtocol {
			t.Fatalf("malformed byte %d accepted: %v", offset, err)
		}
	}
	partial := base[:len(base)-1]
	if _, err := readRequest(bytes.NewReader(partial)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("partial frame accepted")
	}
}

func TestUnknownWireResultDoesNotClaimNotApplied(t *testing.T) {
	for _, err := range []error{errors.New("private detail"), &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1, Inserted: 1}} {
		certainty, _, _ := describeFailure(err)
		if certainty != 3 {
			t.Fatal("unknown outcome downgraded")
		}
	}
	ack := response{sequence: 42, op: opMove, status: 1, certainty: 3}
	var out bytes.Buffer
	if err := writeResponse(&out, ack); err != nil || out.Len() != responseSize || binary.BigEndian.Uint64(out.Bytes()[8:16]) != 42 {
		t.Fatal("response framing failed")
	}
}
