package inputworker

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

type loginNativeFake struct {
	pressErr, releaseErr             error
	presses, releases, moves, cursor int
	move                             func(context.Context) error
}

func (p *loginNativeFake) Press(context.Context, control.PressedToken) error {
	p.presses++
	return p.pressErr
}
func (p *loginNativeFake) Release(context.Context, control.PressedToken) error {
	p.releases++
	return p.releaseErr
}
func (p *loginNativeFake) Move(ctx context.Context, _, _ int32) error {
	p.moves++
	if p.move != nil {
		return p.move(ctx)
	}
	return nil
}
func (p *loginNativeFake) Wheel(context.Context, int32, int32) error { return nil }
func (p *loginNativeFake) CursorPosition(context.Context) (int32, int32, error) {
	p.cursor++
	return 12, 34, nil
}
func (p *loginNativeFake) SetCursorPosition(context.Context, int32, int32) error {
	p.cursor++
	return nil
}
func (p *loginNativeFake) PlaceCursor(context.Context, int32, int32) (application.CursorPlacement, error) {
	p.cursor++
	return application.CursorPlacement{TargetX: 12, X: 12}, nil
}
func (p *loginNativeFake) ProbeCursorPlacement(ctx context.Context, x, y int32) (application.CursorPlacement, error) {
	return p.PlaceCursor(ctx, x, y)
}

func fakeLoginBind(do executor) func(context.Context, func(executor) error) error {
	return func(_ context.Context, callback func(executor) error) (err error) {
		defer func() {
			if recover() != nil {
				err = inputdesktop.ErrCallbackPanic
			}
		}()
		return callback(do)
	}
}

func loginTestDo(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation()
}

var loginTestKey = control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}

func TestLoginInputSerializesCursorAndRecoversOnlyOwnedInputs(t *testing.T) {
	native := &loginNativeFake{}
	checks := 0
	report, err := runLoginInput(context.Background(), native, fakeLoginBind(func(ctx context.Context, op func() error) error {
		checks++
		return loginTestDo(ctx, op)
	}), func(ctx context.Context, port application.InputPort) error {
		if err := port.Press(ctx, loginTestKey); err != nil {
			return err
		}
		if err := port.Release(ctx, control.PressedToken{Kind: control.PressedMouseButton, Code: 1}); err != nil {
			return err
		}
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := port.Move(ctx, 1, 2); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		position := port.(application.CursorPositionPort)
		x, y, err := position.CursorPosition(ctx)
		if err != nil || x != 12 || y != 34 {
			t.Errorf("cursor: %d %d %v", x, y, err)
		}
		if err := position.SetCursorPosition(ctx, 12, 34); err != nil {
			return err
		}
		if _, _, err := port.(application.PositionedMovePort).MoveWithPosition(ctx, 1, 2); err != nil {
			return err
		}
		if _, err := port.(application.CursorPlacementPort).PlaceCursor(ctx, 12, 34); err != nil {
			return err
		}
		_, err = port.(application.CursorPlacementProbePort).ProbeCursorPlacement(ctx, 12, 34)
		return err
	})
	if err != nil || native.presses != 1 || native.releases != 1 || native.moves != 21 || native.cursor != 5 || checks != 28 || report.Recovery.Remaining != 0 || report.Recovery.Released != 1 {
		t.Fatalf("owner: err=%v native=%+v checks=%d recovery=%+v", err, native, checks, report.Recovery)
	}
}

func TestLoginInputCancellationAfterAcceptancePreservesActualOutcome(t *testing.T) {
	actual := errors.New("native completed")
	started, complete := make(chan struct{}), make(chan struct{})
	native := &loginNativeFake{move: func(context.Context) error { close(started); <-complete; return actual }}
	_, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(ctx context.Context, port application.InputPort) error {
		operation, cancel := context.WithCancel(ctx)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- port.Move(operation, 1, 2) }()
		<-started
		cancel()
		select {
		case err := <-result:
			t.Fatalf("accepted operation returned early: %v", err)
		default:
		}
		close(complete)
		if err := <-result; err != actual {
			t.Fatalf("native outcome changed: %v", err)
		}
		return nil
	})
	if err != nil || native.moves != 1 {
		t.Fatalf("err=%v moves=%d", err, native.moves)
	}
}

func TestLoginInputCanceledConnectionAllowsOnlyOwnedRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	native := &loginNativeFake{}
	_, err := runLoginInput(ctx, native, fakeLoginBind(loginTestDo), func(lifetime context.Context, port application.InputPort) error {
		if err := port.Press(lifetime, loginTestKey); err != nil {
			return err
		}
		cancel()
		if err := port.Move(context.Background(), 1, 2); !errors.Is(err, context.Canceled) {
			t.Errorf("new input after cancel: %v", err)
		}
		return port.Release(context.Background(), loginTestKey)
	})
	if err != nil || native.moves != 0 || native.releases != 1 {
		t.Fatalf("err=%v native=%+v", err, native)
	}
}

func TestLoginInputInactiveDesktopDoesNotFakeRecovery(t *testing.T) {
	active := true
	native := &loginNativeFake{}
	report, err := runLoginInput(context.Background(), native, fakeLoginBind(func(ctx context.Context, op func() error) error {
		if !active {
			return inputdesktop.ErrInactive
		}
		return loginTestDo(ctx, op)
	}), func(ctx context.Context, port application.InputPort) error {
		if err := port.Press(ctx, loginTestKey); err != nil {
			return err
		}
		active = false
		failure := port.Move(ctx, 1, 2)
		certainty, _ := application.InjectionCertaintyOf(failure)
		if !errors.Is(failure, inputdesktop.ErrInactive) || certainty != application.InjectionNotApplied {
			t.Errorf("inactive input: %v", failure)
		}
		return nil
	})
	if !errors.Is(err, ErrRecovery) || report.Recovery.Remaining != 1 || native.releases != 0 || native.moves != 0 {
		t.Fatalf("false recovery: err=%v report=%+v native=%+v", err, report, native)
	}
}

func TestLoginInputPressCertaintyDeterminesRecoveryJournal(t *testing.T) {
	for _, certainty := range []application.InjectionCertainty{application.InjectionNotApplied, application.InjectionOutcomeUnknown} {
		t.Run(string(certainty), func(t *testing.T) {
			native := &loginNativeFake{pressErr: &application.InjectionError{Certainty: certainty}, releaseErr: errors.New("release refused")}
			report, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(ctx context.Context, port application.InputPort) error { return port.Press(ctx, loginTestKey) })
			want := 0
			if certainty == application.InjectionOutcomeUnknown {
				want = 1
			}
			if err == nil || report.Recovery.Remaining != want || native.releases != want || errors.Is(err, ErrRecovery) != (want == 1) {
				t.Fatalf("certainty journal: %v %+v releases=%d", err, report, native.releases)
			}
		})
	}
}

func TestLoginInputRetainedPortRejectsAfterCleanup(t *testing.T) {
	native := &loginNativeFake{}
	var retained application.InputPort
	_, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(_ context.Context, port application.InputPort) error { retained = port; return nil })
	if err != nil {
		t.Fatal(err)
	}
	for range 30 {
		if err := retained.Move(context.Background(), 1, 2); !errors.Is(err, inputdesktop.ErrClosed) {
			t.Fatalf("retained port: %v", err)
		}
	}
	if native.moves != 0 {
		t.Fatal("retired port injected")
	}
}

func TestLoginInputCallbackExitStillRecovers(t *testing.T) {
	for _, exit := range []func(){func() { panic("must not be returned") }, runtime.Goexit} {
		native := &loginNativeFake{}
		report, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(ctx context.Context, port application.InputPort) error {
			if err := port.Press(ctx, loginTestKey); err != nil {
				return err
			}
			exit()
			return nil
		})
		if err == nil || report.Recovery.Released != 1 || native.releases != 1 {
			t.Fatalf("exit recovery: %v %+v", err, report)
		}
	}
}

func TestLoginInputNativePanicJoinsConnectionAndPreservesUnknownOutcome(t *testing.T) {
	native := &loginNativeFake{move: func(context.Context) error { panic("native failed") }}
	joined := false
	_, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(ctx context.Context, port application.InputPort) error {
		defer func() { joined = true }()
		failure := port.Move(ctx, 1, 2)
		certainty, _ := application.InjectionCertaintyOf(failure)
		if certainty != application.InjectionOutcomeUnknown {
			t.Errorf("panic became safe-to-retry: %v", failure)
		}
		<-ctx.Done()
		return nil
	})
	if !errors.Is(err, inputdesktop.ErrCallbackPanic) || !joined {
		t.Fatalf("panic did not join connection: %v joined=%v", err, joined)
	}
}
