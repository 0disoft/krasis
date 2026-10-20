package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
)

// Fixed-mode child fixture exercises real inherited stdio/process retirement.
// It performs no native input and never stands in for physical-device evidence.
func TestMain(m *testing.M) {
	if os.Getenv("KRASIS_PORTABLE_RPC_FIXTURE") != "" && len(os.Args) >= 2 {
		input := &portableFixture{x: 10, y: 20, mode: os.Getenv("KRASIS_PORTABLE_RPC_FIXTURE")}
		var err error
		switch os.Args[1] {
		case "product-portable-input-internal":
			_, err = crashguard.ServeWithInput(os.Stdin, os.Stdout, input, PortableInputHandler(input, func(_ context.Context, work func() error) error { return work() }))
		case "product-crash-guard-internal":
			_, err = crashguard.ServeWithResult(os.Stdin, os.Stdout, input)
		case "product-portable-guardian-internal":
			err = servePortableFixtureGuardian(os.Args[2:], input)
		default:
			os.Exit(2)
		}
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type portableFixture struct {
	x, y int32
	mode string
	hits int
}

func (p *portableFixture) Press(context.Context, control.PressedToken) error {
	p.hits++
	if p.mode == "not_applied" {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1}
	}
	return nil
}
func (p *portableFixture) Release(context.Context, control.PressedToken) error {
	p.hits++
	if marker := os.Getenv("KRASIS_PORTABLE_RELEASE_MARKER"); marker != "" {
		return os.WriteFile(marker, []byte("released"), 0o600)
	}
	return nil
}
func (p *portableFixture) Move(_ context.Context, x, y int32) error {
	p.hits++
	if p.mode == "hang" {
		time.Sleep(time.Minute)
	} else if p.mode == "lost_ack" {
		os.Exit(0)
	}
	p.x, p.y = p.x+x, p.y+y
	return nil
}
func (p *portableFixture) Wheel(context.Context, int32, int32) error {
	p.hits++
	if p.mode == "partial" {
		return &application.InjectionError{Certainty: application.InjectionPartiallyApplied, Requested: 2, Inserted: 1}
	}
	return nil
}
func (p *portableFixture) CursorPosition(context.Context) (int32, int32, error) { return p.x, p.y, nil }
func (p *portableFixture) SetCursorPosition(_ context.Context, x, y int32) error {
	p.x, p.y = x, y
	return nil
}
func (p *portableFixture) PlaceCursor(_ context.Context, x, y int32) (application.CursorPlacement, error) {
	p.x, p.y = x, y
	return application.CursorPlacement{TargetX: x, TargetY: y, X: x, Y: y}, nil
}

func startPortableFixture(t *testing.T, mode string) (*crashguard.Process, *Client) {
	t.Helper()
	t.Setenv("KRASIS_PORTABLE_RPC_FIXTURE", mode)
	p, err := crashguard.StartPortable(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	client, err := NewPortableClient(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return p, client
}

func TestPortableRPCCarriesInputCursorTransactionsAndGuardCommands(t *testing.T) {
	p, client := startPortableFixture(t, "normal")
	ctx := context.Background()
	if x, y, err := client.MoveWithPosition(ctx, 3, 4); err != nil || x != 13 || y != 24 {
		t.Fatal("positioned move lost its child readback", x, y, err)
	}
	if placement, err := client.PlaceCursor(ctx, 50, 60); err != nil || placement.X != 50 || placement.TargetY != 60 {
		t.Fatal(placement, err)
	}
	if entry, err := client.EnterCursor(ctx, 70, 80); err != nil || entry.BeforeX != 50 || entry.BeforeY != 60 || entry.X != 70 || entry.TargetY != 80 {
		t.Fatal("cursor entry snapshot/placement crossed operations", entry, err)
	}
	if err := client.SetCursorPosition(ctx, 2, 3); err != nil {
		t.Fatal(err)
	}
	if x, y, err := client.CursorPosition(ctx); err != nil || x != 2 || y != 3 {
		t.Fatal(x, y, err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if prepared, err := p.PreparePress(key); err != nil || !prepared {
		t.Fatal("guard/input stdio commands lost serialization", prepared, err)
	}
	if err := client.Press(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := client.Release(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := p.Released(key); err != nil {
		t.Fatal(err)
	}
	if err := client.Wheel(ctx, 120, -120); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal("portable process did not finish its owned retirement", err)
	}
	if err := client.Move(ctx, 1, 0); err == nil {
		t.Fatal("retired child accepted input")
	}
}

func TestPortableRPCPreservesKnownFailureCertaintyAndGuardCancellation(t *testing.T) {
	for _, mode := range []string{"not_applied", "partial"} {
		t.Run(mode, func(t *testing.T) {
			p, client := startPortableFixture(t, mode)
			key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			if _, err := p.PreparePress(key); err != nil {
				t.Fatal(err)
			}
			want := application.InjectionNotApplied
			err := client.Press(context.Background(), key)
			if mode == "partial" {
				want = application.InjectionPartiallyApplied
				if err != nil {
					t.Fatal(err)
				}
				err = client.Wheel(context.Background(), 120, 120)
			}
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != want {
				t.Fatal("failure certainty changed across ordinary child IPC", err, certainty)
			}
			if mode == "not_applied" {
				if err := p.CancelPreparedPress(key); err != nil {
					t.Fatal("known-unapplied press could not cancel its journal", err)
				}
			}
			if client.Move(context.Background(), 1, 0) == nil {
				t.Fatal("failed client reused its input stream")
			}
			if err := p.Close(); err != nil {
				t.Fatal("failure prevented independent journal disarm", err)
			}
		})
	}
}

func TestPortableRPCLostAckOrNonCooperativeInputCannotBeReplayed(t *testing.T) {
	for _, mode := range []string{"hang", "lost_ack"} {
		t.Run(mode, func(t *testing.T) {
			p, client := startPortableFixture(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			started := time.Now()
			err := client.Move(ctx, 1, 0)
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown || time.Since(started) > time.Second {
				t.Fatal("lost native receipt defeated the caller budget or claimed non-submission", err)
			}
			if client.Move(context.Background(), 2, 0) == nil {
				t.Fatal("unconfirmed operation was replayed")
			}
			if err := p.Close(); err == nil {
				t.Fatal("lost receipt/forced exit became successful cleanup")
			}
			select {
			case <-p.Done():
			default:
				t.Fatal("test input child survived retirement")
			}
		})
	}
}

func TestPortableRPCRejectsOrdinaryGuardAndCanceledAdmission(t *testing.T) {
	t.Setenv("KRASIS_PORTABLE_RPC_FIXTURE", "normal")
	p, err := crashguard.StartProduct(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if client, err := NewPortableClient(context.Background(), p); err == nil || client != nil {
		t.Fatal("ordinary guard impersonated an enabled input child")
	}
	_, client := startPortableFixture(t, "normal")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.Move(ctx, 1, 0)
	if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied {
		t.Fatal("canceled admission lost non-submission certainty", err)
	}
	if err := client.Move(context.Background(), 1, 0); err != nil {
		t.Fatal("unsubmitted cancellation poisoned a fresh call", err)
	}
}

func TestPortableHandlerRejectsReplayForeignBindingAndInvalidLifecycle(t *testing.T) {
	for _, mode := range []string{"replay", "binding", "close", "zero"} {
		t.Run(mode, func(t *testing.T) {
			input := &portableFixture{}
			handle := PortableInputHandler(input, func(_ context.Context, work func() error) error { return work() })
			binding := Binding{1}
			command := request{sequence: 1, binding: binding, op: opHeartbeat}
			data, _ := encodeRequest(command)
			if err := handle(bytes.NewReader(data[:]), io.Discard); err != nil {
				t.Fatal(err)
			}
			command.sequence, command.op, command.x = 2, opMove, 1
			switch mode {
			case "replay":
				command.sequence = 1
			case "binding":
				command.binding[0] = 2
			case "close":
				command.op = opClose
			case "zero":
				command.binding = Binding{}
			}
			data, _ = encodeRequest(command)
			if err := handle(bytes.NewReader(data[:]), io.Discard); !errors.Is(err, ErrProtocol) || input.hits != 0 {
				t.Fatal("invalid local binding/lifecycle reached native input", err)
			}
		})
	}
}

func TestPortableHandlerScopeFailureNeverDispatchesOrReusesInput(t *testing.T) {
	input := &portableFixture{}
	handle := PortableInputHandler(input, func(context.Context, func() error) error { return context.Canceled })
	for sequence := uint64(1); sequence <= 2; sequence++ {
		command := request{sequence: sequence, binding: Binding{1}, op: opPress, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}}
		data, err := encodeRequest(command)
		if err != nil {
			t.Fatal(err)
		}
		var result bytes.Buffer
		if err := handle(bytes.NewReader(data[:]), &result); err != nil {
			t.Fatal(err)
		}
		ack, err := readResponse(&result)
		if err != nil || ack.status != 1 || ack.certainty != 1 || input.hits != 0 {
			t.Fatal("desktop rejection dispatched input or lost not-applied certainty", ack, err)
		}
	}
}

type portablePairReceiptFixture struct {
	pending *crashguard.ExitPending
}

func (fixture *portablePairReceiptFixture) Close() error { return fixture.pending }
func (fixture *portablePairReceiptFixture) ExchangeInput(_ context.Context, exchange func(io.Writer, io.Reader) error) error {
	var input, ack bytes.Buffer
	if err := writeResponse(&ack, response{sequence: 1, op: opHeartbeat}); err != nil {
		return err
	}
	if err := exchange(&input, &ack); err != nil {
		return err
	}
	command, err := readRequest(&input)
	if err != nil || command.op != opHeartbeat {
		return ErrProtocol
	}
	return nil
}

func TestPortableClientCloseRetainsFullPairReceipt(t *testing.T) {
	done := make(chan struct{})
	fixture := &portablePairReceiptFixture{pending: &crashguard.ExitPending{Done: done, Cause: crashguard.ErrExitTimeout}}
	client, err := newPortableClient(context.Background(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	var pending *crashguard.ExitPending
	if err := client.Close(); !errors.As(err, &pending) || pending.Done != done {
		t.Fatal("input client narrowed full pair ownership to one child", err)
	}
	close(done)
	if client, err := NewPortablePairClient(context.Background(), &crashguard.PortableProcesses{}); err == nil || client != nil {
		t.Fatal("uninitialized process pair was accepted")
	}
}
