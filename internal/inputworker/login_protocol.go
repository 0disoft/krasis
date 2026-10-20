package inputworker

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const loginControlSize = 80

type loginControlKind byte

const (
	loginChallenge loginControlKind = iota + 1
	loginReady
	loginActivate
	loginStop
	loginClean
	loginFailed
	loginFailedDesktop
	loginFailedPolicy
	loginFailedInput
	loginFailedCapture
	loginFailedCleanup
	loginFailedConnection
)

// No paths, commands, credentials or input values travel on this channel.
// The nonce binds acknowledgements to one authenticated process lifetime.
type loginBinding struct {
	session uint32
	profile [32]byte
	nonce   [32]byte
}

// Only the fixed runtime may use this marker before its input/network callback
// starts. A requested Stop can safely retire such a no-input startup race;
// ordinary runtime errors and desktop cleanup failures remain failures.
type loginPreparationError struct{ cause error }

func (e *loginPreparationError) Error() string { return "login input preparation stopped" }
func (e *loginPreparationError) Unwrap() error { return e.cause }

func newLoginBinding(session uint32, profile [32]byte) (loginBinding, error) {
	b := loginBinding{session: session, profile: profile}
	if session == 0 || session == ^uint32(0) || profile == ([32]byte{}) {
		return loginBinding{}, ErrAuthority
	}
	if _, err := rand.Read(b.nonce[:]); err != nil || b.nonce == ([32]byte{}) {
		return loginBinding{}, ErrAuthority
	}
	return b, nil
}

func encodeLoginControl(b loginBinding, kind loginControlKind) ([loginControlSize]byte, error) {
	var data [loginControlSize]byte
	if b.session == 0 || b.session == ^uint32(0) || b.profile == ([32]byte{}) || b.nonce == ([32]byte{}) || kind < loginChallenge || kind > loginFailedConnection {
		return data, ErrProtocol
	}
	copy(data[:4], "KRL1")
	data[4], data[5] = 1, byte(kind)
	binary.BigEndian.PutUint32(data[8:12], b.session)
	copy(data[12:44], b.profile[:])
	copy(data[44:76], b.nonce[:])
	return data, nil
}

func readLoginControl(reader io.Reader) (loginBinding, loginControlKind, error) {
	var data [loginControlSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return loginBinding{}, 0, errors.Join(ErrProtocol, err)
	}
	if string(data[:4]) != "KRL1" || data[4] != 1 || !zeroBytes(data[6:8]) || !zeroBytes(data[76:]) {
		return loginBinding{}, 0, ErrProtocol
	}
	b := loginBinding{session: binary.BigEndian.Uint32(data[8:12])}
	copy(b.profile[:], data[12:44])
	copy(b.nonce[:], data[44:76])
	kind := loginControlKind(data[5])
	if _, err := encodeLoginControl(b, kind); err != nil {
		return loginBinding{}, 0, err
	}
	return b, kind, nil
}

func writeLoginControl(writer io.Writer, b loginBinding, kind loginControlKind) error {
	data, err := encodeLoginControl(b, kind)
	if err != nil {
		return err
	}
	return writeFrame(writer, data[:])
}

// Both authenticate callbacks bind the actual kernel pipe process and fixed
// image. The child must prove its server before consuming a challenge; the
// service must prove its exact owned child before sending anything.
func loginParentBootstrap(ctx context.Context, stream net.Conn, binding loginBinding, authenticate func() error, beforeActivate ...func()) error {
	if ctx == nil || ctx.Err() != nil || stream == nil || authenticate == nil || authenticate() != nil {
		return ErrAuthority
	}
	if err := writeLoginControl(stream, binding, loginChallenge); err != nil {
		return err
	}
	actual, kind, err := readLoginControl(stream)
	if err != nil || actual != binding || kind != loginReady {
		return ErrProtocol
	}
	if ctx.Err() != nil || authenticate() != nil {
		return ErrAuthority
	}
	for _, notify := range beforeActivate {
		if notify != nil {
			notify()
		}
	}
	if err := writeLoginControl(stream, binding, loginActivate); err != nil {
		// A failed write does not prove that the child did not activate.
		return errors.Join(ErrRecovery, err)
	}
	return nil
}

func loginChildBootstrap(ctx context.Context, stream net.Conn, authenticate func() error, authorize func(loginBinding) error) (loginBinding, error) {
	if ctx == nil || ctx.Err() != nil || stream == nil || authenticate == nil || authorize == nil || authenticate() != nil {
		return loginBinding{}, ErrAuthority
	}
	binding, kind, err := readLoginControl(stream)
	if err != nil || kind != loginChallenge {
		return loginBinding{}, ErrProtocol
	}
	if ctx.Err() != nil || authenticate() != nil || authorize(binding) != nil {
		return loginBinding{}, ErrAuthority
	}
	if err := writeLoginControl(stream, binding, loginReady); err != nil {
		return loginBinding{}, err
	}
	actual, kind, err := readLoginControl(stream)
	if err != nil || actual != binding || kind != loginActivate {
		return loginBinding{}, ErrProtocol
	}
	if ctx.Err() != nil || authenticate() != nil || authorize(binding) != nil {
		return loginBinding{}, ErrAuthority
	}
	return binding, nil
}

// A clean acknowledgement is sent only after run and all its input/network
// cleanup returned successfully. A lost pipe or malformed stop still cancels
// the runtime, but never becomes evidence of clean retirement.
func serveLoginControl(ctx context.Context, stream net.Conn, binding loginBinding, authenticate func() error, run func(context.Context) error) (result error) {
	if ctx == nil || ctx.Err() != nil || stream == nil || authenticate == nil || run == nil || authenticate() != nil {
		return ErrAuthority
	}
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	var requested bool
	var readErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		actual, kind, err := readLoginControl(stream)
		// This exact already-authenticated stream can only revoke authority.
		// Requiring SERVICE_RUNNING here would reject SCM StopPending and
		// prevent an orderly shutdown. It cannot activate or extend a runtime.
		if err == nil && actual == binding && kind == loginStop {
			requested = true
		} else {
			readErr = errors.Join(ErrProtocol, err)
		}
		cancel()
	}()
	result = run(lifetime)
	failureKind := loginFailureKind(result)
	// Joining the reader prevents a late command from acting on a new owner.
	// Desktop/policy observation can stop the runtime just before the parent's
	// next console tick. Allow that exact authenticated parent a bounded chance
	// to revoke and confirm cleanup; an absent or wrong stop remains a failure.
	if err := stream.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		_ = stream.Close()
	}
	<-done
	if !requested {
		result = errors.Join(result, readErr, ErrRecovery)
	} else if _, noInput := result.(*loginPreparationError); noInput {
		result = nil
	}
	if err := stream.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return errors.Join(result, ErrRecovery)
	}
	kind := loginClean
	if result != nil {
		kind = failureKind
	}
	if err := writeLoginControl(stream, binding, kind); err != nil {
		return errors.Join(result, ErrRecovery)
	}
	return result
}

type loginControlWorker struct {
	stream  net.Conn
	binding loginBinding
	cancel  context.CancelFunc
	done    chan struct{}
	err     error // read only after done closes
}

// reap owns the exact process/job and must join their bounded cleanup. Process
// exit by itself is never a clean input-retirement receipt.
func startLoginControlWorker(parent context.Context, stream net.Conn, binding loginBinding, reap func() error, clean ...func()) *loginControlWorker {
	lifetime, cancel := context.WithCancel(parent)
	w := &loginControlWorker{stream: stream, binding: binding, cancel: cancel, done: make(chan struct{})}
	stopFinished := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			defer close(stopFinished)
			_ = stream.SetWriteDeadline(time.Now().Add(time.Second))
			_ = stream.SetReadDeadline(time.Now().Add(3 * time.Second))
			if writeLoginControl(stream, binding, loginStop) != nil {
				_ = stream.Close()
			}
		})
	}
	stopCancellation := context.AfterFunc(lifetime, stop)
	go func() {
		defer close(w.done)
		actual, kind, err := readLoginControl(stream)
		if err != nil || actual != binding || kind != loginClean {
			w.err = ErrRecovery
			if err == nil && actual == binding && kind >= loginFailed && kind <= loginFailedConnection {
				w.err = errors.Join(w.err, &loginRuntimeFailure{reason: loginFailureReason(kind), cause: ErrRecovery})
			}
		} else {
			for _, notify := range clean {
				if notify != nil {
					notify()
				}
			}
		}
		cancel()
		if !stopCancellation() {
			<-stopFinished
		}
		w.err = errors.Join(w.err, stream.Close(), reap())
	}()
	return w
}

func (w *loginControlWorker) Done() <-chan struct{} { return w.done }

func (w *loginControlWorker) Stop(ctx context.Context) error {
	if w == nil || ctx == nil {
		return ErrRecovery
	}
	w.cancel()
	select {
	case <-w.done:
		return w.err
	case <-ctx.Done():
		_ = w.stream.Close()
		return errors.Join(ErrRecovery, ctx.Err())
	}
}
