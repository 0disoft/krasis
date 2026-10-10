package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func loginTestBinding(t *testing.T) loginBinding {
	t.Helper()
	b, err := newLoginBinding(1, [32]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoginControlRejectsMalformedSelectorsAndExtraFields(t *testing.T) {
	b := loginTestBinding(t)
	data, err := encodeLoginControl(b, loginChallenge)
	if err != nil {
		t.Fatal(err)
	}
	actual, kind, err := readLoginControl(bytes.NewReader(data[:]))
	if err != nil || actual != b || kind != loginChallenge {
		t.Fatal("binding roundtrip failed", err)
	}
	for _, index := range []int{0, 4, 5, 6, 7, 76, 79} {
		other := data
		other[index] = 255
		if _, _, err := readLoginControl(bytes.NewReader(other[:])); err == nil {
			t.Fatal("malformed control frame accepted", index)
		}
	}
	for _, invalid := range []loginBinding{{}, {session: 1, profile: b.profile}, {session: ^uint32(0), profile: b.profile, nonce: b.nonce}} {
		if _, err := encodeLoginControl(invalid, loginActivate); err == nil {
			t.Fatal("invalid process binding accepted")
		}
	}
	if _, _, err := readLoginControl(bytes.NewReader(data[:len(data)-1])); err == nil {
		t.Fatal("truncated frame accepted")
	}
}

func TestLoginBootstrapAuthenticatesBeforeReadingOrWriting(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	b := loginTestBinding(t)
	deny := func() error { return ErrAuthority }
	if err := loginParentBootstrap(context.Background(), left, b, deny); !errors.Is(err, ErrAuthority) {
		t.Fatal(err)
	}
	if _, err := loginChildBootstrap(context.Background(), right, deny, func(loginBinding) error { t.Fatal("unauthenticated frame reached policy"); return nil }); !errors.Is(err, ErrAuthority) {
		t.Fatal(err)
	}
}

func TestLoginBootstrapRechecksPolicyBeforeActivation(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = left.SetDeadline(time.Now().Add(time.Second))
	_ = right.SetDeadline(time.Now().Add(time.Second))
	var checks int
	denySecond := func() error {
		checks++
		if checks == 2 {
			return ErrAuthority
		}
		return nil
	}
	childDone := make(chan error, 1)
	go func() {
		_, err := loginChildBootstrap(ctx, right, func() error { return nil }, func(loginBinding) error { return nil })
		childDone <- err
	}()
	if err := loginParentBootstrap(ctx, left, loginTestBinding(t), denySecond); !errors.Is(err, ErrAuthority) {
		t.Fatal("stale parent policy activated child", err)
	}
	_ = left.Close()
	if err := <-childDone; err == nil {
		t.Fatal("child accepted missing activation")
	}
}

func TestLoginBootstrapRejectsReceiptFromAnotherLifetime(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	_ = left.SetDeadline(time.Now().Add(time.Second))
	_ = right.SetDeadline(time.Now().Add(time.Second))
	b := loginTestBinding(t)
	done := make(chan error, 1)
	go func() {
		actual, _, err := readLoginControl(right)
		if err == nil {
			actual.nonce[0] ^= 1
			err = writeLoginControl(right, actual, loginReady)
		}
		done <- err
	}()
	if err := loginParentBootstrap(context.Background(), left, b, func() error { return nil }); !errors.Is(err, ErrProtocol) {
		t.Fatal("wrong lifetime receipt accepted", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLoginStopWaitsForInputCleanupAndExactProcessReap(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	b := loginTestBinding(t)
	started := make(chan struct{})
	cleanupStarted := make(chan struct{})
	allowCleanup := make(chan struct{})
	childDone := make(chan error, 1)
	go func() {
		childDone <- serveLoginControl(context.Background(), right, b, func() error { return nil }, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(cleanupStarted)
			<-allowCleanup
			return nil
		})
	}()
	var reaped atomic.Bool
	worker := startLoginControlWorker(context.Background(), left, b, func() error { reaped.Store(true); return nil })
	<-started
	worker.cancel()
	<-cleanupStarted
	select {
	case <-worker.Done():
		t.Fatal("retirement reported before input cleanup")
	default:
	}
	if reaped.Load() {
		t.Fatal("live cleanup worker killed early")
	}
	close(allowCleanup)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Stop(ctx); err != nil {
		t.Fatal("clean retirement refused", err)
	}
	if !reaped.Load() {
		t.Fatal("exact owned process not reaped")
	}
	if err := <-childDone; err != nil {
		t.Fatal(err)
	}
}

func TestLoginStopPreservesCleanupFailureAndLostPipe(t *testing.T) {
	for _, lost := range []bool{false, true} {
		left, right := net.Pipe()
		b := loginTestBinding(t)
		childDone := make(chan struct{})
		if lost {
			_ = right.Close()
			close(childDone)
		} else {
			go func() {
				defer close(childDone)
				_ = serveLoginControl(context.Background(), right, b, func() error { return nil }, func(ctx context.Context) error { <-ctx.Done(); return ErrRecovery })
			}()
		}
		worker := startLoginControlWorker(context.Background(), left, b, func() error { return nil })
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := worker.Stop(ctx); !errors.Is(err, ErrRecovery) {
			t.Fatal("uncertain cleanup marked clean", err)
		}
		cancel()
		_ = left.Close()
		_ = right.Close()
		<-childDone
	}
}

func TestLoginStopCanRevokeWhileSCMLeavesRunningState(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	b := loginTestBinding(t)
	started := make(chan struct{})
	var stopping atomic.Bool
	childDone := make(chan error, 1)
	go func() {
		childDone <- serveLoginControl(context.Background(), right, b, func() error {
			if stopping.Load() {
				return ErrAuthority
			}
			return nil
		}, func(ctx context.Context) error { close(started); <-ctx.Done(); return nil })
	}()
	worker := startLoginControlWorker(context.Background(), left, b, func() error { return nil })
	<-started
	stopping.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Stop(ctx); err != nil {
		t.Fatal("SCM stop prevented authenticated revocation", err)
	}
	if err := <-childDone; err != nil {
		t.Fatal(err)
	}
}

func TestLoginOwnerFencesFailedTerminalReceipt(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	b := loginTestBinding(t)
	worker := startLoginControlWorker(context.Background(), left, b, func() error { return ErrRecovery })
	go func() { _ = writeLoginControl(right, b, loginFailed) }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Stop(ctx); !errors.Is(err, ErrRecovery) {
		t.Fatal("native cleanup failure lost", err)
	}
}

func TestLoginRuntimeCanRetireBeforeNextParentConsoleTick(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	b := loginTestBinding(t)
	runtimeEnded := make(chan struct{})
	childDone := make(chan error, 1)
	go func() {
		childDone <- serveLoginControl(context.Background(), right, b, func() error { return nil }, func(context.Context) error {
			close(runtimeEnded)
			return nil
		})
	}()
	worker := startLoginControlWorker(context.Background(), left, b, func() error { return nil })
	<-runtimeEnded
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Stop(ctx); err != nil {
		t.Fatal("parent tick lost a completed clean retirement", err)
	}
	if err := <-childDone; err != nil {
		t.Fatal(err)
	}
}
