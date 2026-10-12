package inputworker

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
	"math"
	"net"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

const (
	IdleTimeout     = 2 * time.Second
	RecoveryTimeout = 2 * time.Second
	maxPressed      = 256
	maxDelta        = control.MaxInputDelta
)

var (
	ErrAuthority = errors.New("input worker authority unavailable")
	ErrExpired   = errors.New("input worker liveness expired")
	ErrRecovery  = errors.New("input worker release recovery failed")
)

// Binding is an unpredictable per-lease channel identity. It never substitutes
// for Windows process authentication or the resident agent's mTLS authorization.
type Binding [16]byte

func NewBinding() (Binding, error) {
	var value Binding
	_, err := rand.Read(value[:])
	return value, err
}

// Lease fixes the already-approved remote lease for this worker lifetime. The
// privileged transport must independently authenticate its exact agent process,
// console session and installation before calling Serve. Neither this type nor
// the wire protocol accepts a process, executable, desktop or privilege selector.
type Lease struct {
	Identity  application.LeaseIdentity
	Binding   Binding
	ExpiresAt time.Time
	// Presentation permits only a passive Winlogon cursor shield after a clean
	// lease return. It never admits input requests or a desktop selector.
	Presentation     bool
	Capture          bool // Separate authenticated source-only endpoint; no injection decoder.
	CursorX, CursorY int32
}

type Report struct {
	Completed uint64
	Recovery  application.ReleaseReport
}

type executor func(context.Context, func() error) error

// Serve owns all input and recovery on one OS thread. It consumes one authorized
// lease only and always closes the stream. A missing response is outcome-unknown
// to the caller; callers must not retry an input or switch backends.
// This foundational entry point is not reachable from the product agent yet.
func Serve(ctx context.Context, channel *Channel, input application.InputPort, lease Lease) (report Report, err error) {
	return serveReady(ctx, channel, input, lease, nil)
}

func serveReady(ctx context.Context, channel *Channel, input application.InputPort, lease Lease, ready func() error) (report Report, err error) {
	return serveReadyProfile(ctx, channel, input, lease, ready, nil)
}

func serveReadyProfile(ctx context.Context, channel *Channel, input application.InputPort, lease Lease, ready func() error, profile *handoff.StartupProfile) (report Report, err error) {
	return serveReadyProfileOnDesktop(ctx, channel, input, lease, ready, profile, inputdesktop.Run)
}

type desktopRunner func(context.Context, func(*inputdesktop.Scope) error) error

func serveReadyProfileOnDesktop(ctx context.Context, channel *Channel, input application.InputPort, lease Lease, ready func() error, profile *handoff.StartupProfile, desktop desktopRunner) (report Report, err error) {
	if channel == nil || desktop == nil {
		return report, ErrAuthority
	}
	stream, authority := channel.stream, channel.authority
	defer channel.Close()
	if !validLease(ctx, stream, input, lease, time.Now()) || authority == nil || authority.kind != authorityAgent || authority.check(false) != nil {
		if stream != nil {
			_ = stream.Close()
		}
		return report, ErrAuthority
	}
	// Root ownership stays alive during independent key-up recovery after ctx
	// cancellation. The loop itself observes ctx and the immutable lease expiry.
	desktopStarted := time.Now()
	err = desktop(context.Background(), func(scope *inputdesktop.Scope) error {
		if ready != nil {
			if err := scope.Do(ctx, func() error {
				if authority.check(false) != nil {
					return ErrAuthority
				}
				profile.Record(handoff.WorkerDesktopReady, time.Since(desktopStarted))
				return ready()
			}); err != nil {
				return err
			}
		}
		var runErr error
		timing := &workerTiming{}
		inputDo := beforeInputCertainty(measureExecution(func() error { return authority.check(false) }, scope.Do, timing))
		recoveryDo := func(ctx context.Context, operation func() error) error {
			if err := authority.check(true); err != nil {
				return err
			}
			return scope.Do(ctx, operation)
		}
		report, runErr = serve(ctx, stream, input, lease, inputDo, recoveryDo, time.Now, timing)
		return runErr
	})
	return report, err
}

func validLease(ctx context.Context, stream net.Conn, input application.InputPort, lease Lease, now time.Time) bool {
	if lease.Presentation {
		return false
	}
	if ctx == nil || stream == nil || input == nil || ctx.Err() != nil || !lease.ExpiresAt.After(now) || lease.Binding == (Binding{}) {
		return false
	}
	for _, id := range []string{lease.Identity.SessionID, lease.Identity.LeaseID, lease.Identity.Epoch} {
		if len(id) == 0 || len(id) > 128 {
			return false
		}
	}
	return true
}

func serve(ctx context.Context, stream net.Conn, input application.InputPort, lease Lease, do, recoveryDo executor, now func() time.Time, timings ...*workerTiming) (report Report, err error) {
	var pressed control.PressedState
	defer func() {
		// Close first to interrupt the sole bounded reader. Cleanup cannot wait
		// for the client to read an acknowledgement or send any further command.
		_ = stream.Close()
		if pressed.Count() == 0 {
			return
		}
		recoveryCtx, cancel := context.WithTimeout(context.Background(), RecoveryTimeout)
		defer cancel()
		recoveryErr := recoveryDo(recoveryCtx, func() error {
			var releaseErr error
			report.Recovery, releaseErr = application.ReleaseAll(recoveryCtx, &pressed, input)
			return releaseErr
		})
		if recoveryErr != nil {
			report.Recovery.Remaining = pressed.Count()
			err = errors.Join(err, ErrRecovery)
		}
	}()
	// Read on the desktop-owning thread: a separate reader and rendezvous
	// require another scheduler handoff for every serialized input. The stream
	// deadline bounds even a partial frame; cancellation closes pending I/O.
	stopCancel := context.AfterFunc(ctx, func() { _ = stream.Close() })
	defer stopCancel()
	sequence := uint64(1)
	deadline := now().Add(IdleTimeout)
	if lease.ExpiresAt.Before(deadline) {
		deadline = lease.ExpiresAt
	}
	for {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if !now().Before(deadline) {
			return report, ErrExpired
		}
		if err := stream.SetReadDeadline(deadline); err != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			// A peer can close just after the previous ACK, before the next
			// deadline is installed. Preserve normal EOF cleanup in that race.
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
				return report, nil
			}
			return report, ErrProtocol
		}
		command, readErr := readRequest(stream)
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if readErr != nil {
			var timeout net.Error
			if errors.As(readErr, &timeout) && timeout.Timeout() {
				return report, ErrExpired
			}
			if errors.Is(readErr, io.EOF) {
				return report, nil
			}
			return report, ErrProtocol
		}
		if !now().Before(deadline) {
			return report, ErrExpired
		}
		if command.sequence != sequence || sequence == math.MaxUint64 ||
			subtle.ConstantTimeCompare(command.binding[:], lease.Binding[:]) != 1 {
			return report, ErrProtocol
		}
		sequence++
		if command.op == opClose {
			// Close has no optimistic acknowledgement: connection teardown
			// is not proof that recovery succeeded. Report remains supervisor-owned.
			return report, nil
		}
		operationCtx, cancel := context.WithDeadline(ctx, deadline)
		ack := response{sequence: command.sequence, op: command.op}
		operationErr := do(operationCtx, func() error {
			return applyCommand(operationCtx, input, &pressed, command, &ack)
		})
		cancel()
		if len(timings) == 1 && timings[0] != nil {
			ack.timing = timings[0].buckets
			ack.millis, ack.measured = timings[0].millis, timings[0].measured
		}
		if operationErr != nil {
			ack.status = 1
			ack.certainty, ack.requested, ack.inserted = describeFailure(operationErr)
		} else if command.op != opHeartbeat {
			report.Completed++
		}
		if deadlineErr := stream.SetWriteDeadline(deadline); deadlineErr != nil {
			return report, ErrProtocol
		}
		if writeErr := writeResponse(stream, ack); writeErr != nil {
			return report, ErrProtocol
		}
		if operationErr != nil {
			return report, operationFailure(operationErr)
		}
		deadline = now().Add(IdleTimeout)
		if lease.ExpiresAt.Before(deadline) {
			deadline = lease.ExpiresAt
		}
	}
}

func apply(ctx context.Context, input application.InputPort, pressed *control.PressedState, command request) error {
	switch command.op {
	case opPress:
		if !pressed.Contains(command.token) && pressed.Count() >= maxPressed {
			return ErrProtocol
		}
		return application.InjectPress(ctx, pressed, input, command.token)
	case opRelease:
		_, err := application.InjectRelease(ctx, pressed, input, command.token)
		return err
	case opMove, opWheel, opMoveRead:
		if command.x < -maxDelta || command.x > maxDelta || command.y < -maxDelta || command.y > maxDelta {
			return ErrProtocol
		}
		if command.op == opMove || command.op == opMoveRead {
			return input.Move(ctx, command.x, command.y)
		}
		return input.Wheel(ctx, command.x, command.y)
	case opHeartbeat:
		return nil
	case opCursorSet:
		port, ok := input.(application.CursorPositionPort)
		if !ok || pressed.Count() != 0 {
			return notSent(ErrProtocol)
		}
		return port.SetCursorPosition(ctx, command.x, command.y)
	default:
		return ErrProtocol
	}
}

func describeFailure(err error) (certainty byte, requested, inserted uint16) {
	var failure *application.InjectionError
	if !errors.As(err, &failure) || failure == nil {
		return 3, 0, 0
	}
	switch failure.Certainty {
	case application.InjectionNotApplied:
		certainty = 1
	case application.InjectionPartiallyApplied:
		certainty = 2
	default:
		certainty = 3
	}
	if failure.Requested >= 0 && failure.Requested <= math.MaxUint16 && failure.Inserted >= 0 && failure.Inserted <= failure.Requested {
		if certainty == 1 && failure.Inserted != 0 {
			return 3, 0, 0
		}
		if certainty == 2 && (failure.Inserted == 0 || failure.Inserted == failure.Requested) {
			return 3, 0, 0
		}
		return certainty, uint16(failure.Requested), uint16(failure.Inserted)
	}
	return 3, 0, 0
}

func operationFailure(err error) error {
	certainty, requested, inserted := describeFailure(err)
	if certainty == 3 {
		return &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}
	}
	value := application.InjectionNotApplied
	if certainty == 2 {
		value = application.InjectionPartiallyApplied
	}
	return &application.InjectionError{Certainty: value, Requested: int(requested), Inserted: int(inserted)}
}
