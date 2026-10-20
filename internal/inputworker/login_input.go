package inputworker

import (
	"context"
	"errors"
	"sync"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

type loginNativeInput interface {
	application.InputPort
	application.CursorPositionPort
	application.CursorPlacementPort
	application.CursorPlacementProbePort
}

type loginInputRequest struct {
	ctx       context.Context
	recovery  bool
	operation func(*control.PressedState) error
	result    chan error
}

// The unbuffered mailbox admits one operation at a time. An accepted operation
// always returns its actual outcome, even if its caller cancels while waiting.
// Only key/button releases may be admitted after the connection is canceled.
type loginInput struct {
	root    context.Context
	native  loginNativeInput
	mailbox chan loginInputRequest
	done    chan struct{}
	retired chan struct{}
	close   sync.Once
}

func (port *loginInput) stop() { port.close.Do(func() { close(port.done) }) }

func runLoginInput(ctx context.Context, native loginNativeInput,
	bind func(context.Context, func(executor) error) error,
	callback func(context.Context, application.InputPort) error) (report Report, result error) {
	if ctx == nil || native == nil || bind == nil || callback == nil {
		return report, ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	port := &loginInput{root: lifetime, native: native, mailbox: make(chan loginInputRequest), done: make(chan struct{}), retired: make(chan struct{})}
	defer port.stop()
	// Keep the desktop ownership alive for independent release recovery. The
	// connection callback must cooperatively join its own cancellation cleanup.
	result = bind(context.Background(), func(do executor) (err error) {
		liveDo := journalExecution(native, do, nil)
		var pressed control.PressedState
		finished := port.retired
		callbackErr := inputdesktop.ErrCallbackExit
		defer func() {
			cancel()
			port.stop()
			// Even an input panic or Goexit must retire the connection callback
			// before scope cleanup. Its pending calls now receive an explicit
			// closed/unknown outcome instead of deadlocking this owner.
			<-finished
			err = errors.Join(err, callbackErr)
			if owner, ok := native.(interface{ retireForRecovery() (bool, error) }); ok {
				retired, retireErr := owner.retireForRecovery()
				err = errors.Join(err, retireErr)
				if !retired {
					report.Recovery.Remaining = pressed.Count()
					err = errors.Join(err, ErrRecovery)
					return
				}
			}
			if pressed.Count() == 0 {
				return
			}
			recovery, cancel := context.WithTimeout(context.Background(), RecoveryTimeout)
			defer cancel()
			releaseErr := do(recovery, func() error {
				var failure error
				report.Recovery, failure = application.ReleaseAll(recovery, &pressed, native)
				return failure
			})
			report.Recovery.Remaining = pressed.Count()
			if releaseErr != nil || pressed.Count() != 0 {
				// Never release on Default or declare a clean handoff after an
				// inactive Winlogon scope refused to release our owned inputs.
				err = errors.Join(err, ErrRecovery, releaseErr)
			}
		}()
		go func() {
			defer func() {
				if recover() != nil {
					callbackErr = inputdesktop.ErrCallbackPanic
				}
				close(finished)
			}()
			callbackErr = callback(lifetime, port)
		}()
		for {
			select {
			case <-finished:
				return nil
			case request := <-port.mailbox:
				started := false
				err := liveDo(request.ctx, func() error {
					select {
					case <-finished:
						return notSent(inputdesktop.ErrClosed)
					default:
					}
					if !request.recovery && lifetime.Err() != nil {
						return notSent(lifetime.Err())
					}
					started = true
					return request.operation(&pressed)
				})
				if err != nil && !started {
					err = notSent(err)
				}
				if err == nil {
					report.Completed++
				}
				request.result <- err
			}
		}
	})
	return report, result
}

func (port *loginInput) do(ctx context.Context, recovery bool, operation func(*control.PressedState) error) error {
	if ctx == nil || operation == nil || port == nil {
		return notSent(ErrAuthority)
	}
	if err := ctx.Err(); err != nil {
		return notSent(err)
	}
	select {
	case <-port.done:
		return notSent(inputdesktop.ErrClosed)
	case <-port.retired:
		return notSent(inputdesktop.ErrClosed)
	default:
	}
	if !recovery && port.root.Err() != nil {
		return notSent(port.root.Err())
	}
	request := loginInputRequest{ctx: ctx, recovery: recovery, operation: operation, result: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return notSent(ctx.Err())
	case <-port.done:
		return notSent(inputdesktop.ErrClosed)
	case <-port.retired:
		return notSent(inputdesktop.ErrClosed)
	case port.mailbox <- request:
	}
	select {
	case err := <-request.result:
		return err
	case <-port.done:
		select {
		case err := <-request.result:
			return err
		default:
			return &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: inputdesktop.ErrClosed}
		}
	}
}

func (port *loginInput) Press(ctx context.Context, token control.PressedToken) error {
	return port.do(ctx, false, func(pressed *control.PressedState) error {
		if err := token.Validate(); err != nil {
			return notSent(err)
		}
		if !pressed.Contains(token) && pressed.Count() >= maxPressed {
			return notSent(ErrProtocol)
		}
		return application.InjectPress(ctx, pressed, port.native, token)
	})
}

func (port *loginInput) Release(ctx context.Context, token control.PressedToken) error {
	return port.do(ctx, true, func(pressed *control.PressedState) error {
		_, err := application.InjectRelease(ctx, pressed, port.native, token)
		return err
	})
}

func (port *loginInput) Move(ctx context.Context, x, y int32) error {
	return port.do(ctx, false, func(*control.PressedState) error {
		if x < -maxDelta || x > maxDelta || y < -maxDelta || y > maxDelta {
			return notSent(ErrProtocol)
		}
		return port.native.Move(ctx, x, y)
	})
}

func (port *loginInput) Wheel(ctx context.Context, x, y int32) error {
	return port.do(ctx, false, func(*control.PressedState) error {
		if x < -maxDelta || x > maxDelta || y < -maxDelta || y > maxDelta {
			return notSent(ErrProtocol)
		}
		return port.native.Wheel(ctx, x, y)
	})
}

func (port *loginInput) CursorPosition(ctx context.Context) (x, y int32, err error) {
	err = port.do(ctx, false, func(*control.PressedState) error {
		x, y, err = port.native.CursorPosition(ctx)
		return err
	})
	return
}

func (port *loginInput) SetCursorPosition(ctx context.Context, x, y int32) error {
	return port.do(ctx, false, func(*control.PressedState) error {
		return port.native.SetCursorPosition(ctx, x, y)
	})
}

func (port *loginInput) MoveWithPosition(ctx context.Context, x, y int32) (px, py int32, err error) {
	err = port.do(ctx, false, func(*control.PressedState) error {
		if x < -maxDelta || x > maxDelta || y < -maxDelta || y > maxDelta {
			return notSent(ErrProtocol)
		}
		if err := port.native.Move(ctx, x, y); err != nil {
			return err
		}
		px, py, err = port.native.CursorPosition(ctx)
		if err != nil {
			return &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: err}
		}
		return nil
	})
	return
}

func (port *loginInput) PlaceCursor(ctx context.Context, x, y int32) (value application.CursorPlacement, err error) {
	err = port.do(ctx, false, func(*control.PressedState) error {
		value, err = port.native.PlaceCursor(ctx, x, y)
		return err
	})
	return
}

func (port *loginInput) ProbeCursorPlacement(ctx context.Context, x, y int32) (value application.CursorPlacement, err error) {
	err = port.do(ctx, false, func(*control.PressedState) error {
		value, err = port.native.ProbeCursorPlacement(ctx, x, y)
		return err
	})
	return
}
