//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestNativePreparationOverlapsAuthenticatedBootstrap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		pair := &preparedNativePair{}
		pending, err := startNativePairPreparation(context.Background(), func(context.Context) (*preparedNativePair, error) {
			time.Sleep(60 * time.Millisecond) // Exact child creation, no input handles.
			return pair, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(40 * time.Millisecond) // Independent service/agent bootstrap.
		ready, err := pending.awaitPreparation(context.Background())
		if err != nil || ready != pair || time.Since(started) != 60*time.Millisecond {
			t.Fatal("native creation serialized bootstrap or published an unfinished pair", time.Since(started), err)
		}
	})
}

func TestNativePreparationCancellationDeniesBindingButJoinsLateConstructor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		started := time.Now()
		pair := &preparedNativePair{}
		pending, err := startNativePairPreparation(ctx, func(context.Context) (*preparedNativePair, error) {
			time.Sleep(60 * time.Millisecond) // OS creation can finish after cancellation.
			return pair, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ready, err := pending.awaitPreparation(ctx)
		if ready != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 20*time.Millisecond {
			t.Fatal("expired caller received a pair or waited for native startup", ready, err)
		}
		if err := pending.Close(); err != nil || time.Since(started) != 60*time.Millisecond {
			t.Fatal("cancellation abandoned exact startup ownership", err, time.Since(started))
		}
		if _, err := pending.awaitPreparation(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("a new context revived expired preparation", err)
		}
	})
}

func TestNativePreparationFailureRetainsCleanupClassification(t *testing.T) {
	for _, cause := range []error{ErrAuthority, errors.Join(ErrAuthority, ErrRecovery), nil} {
		pending, err := startNativePairPreparation(context.Background(), func(context.Context) (*preparedNativePair, error) { return nil, cause })
		if err != nil {
			t.Fatal(err)
		}
		pair, err := pending.awaitPreparation(context.Background())
		if pair != nil || err == nil || cause != nil && !errors.Is(err, cause) {
			t.Fatal("failed or missing pair accepted", pair, err)
		}
		closeErr := pending.Close()
		if errors.Is(closeErr, ErrRecovery) != errors.Is(cause, ErrRecovery) || !errors.Is(cause, ErrRecovery) && closeErr != nil {
			t.Fatal("construction rejection changed retirement certainty", cause, closeErr)
		}
	}
}

func TestNativePreparationFinalReceiptWaitsForConstructorAndRetirement(t *testing.T) {
	for _, cause := range []error{ErrAuthority, ErrRecovery} {
		synctest.Test(t, func(t *testing.T) {
			pending, err := startNativePairPreparation(context.Background(), func(context.Context) (*preparedNativePair, error) {
				time.Sleep(60 * time.Millisecond)
				return nil, cause
			})
			if err != nil {
				t.Fatal(err)
			}
			child, service := net.Pipe()
			defer child.Close()
			defer service.Close()
			done := make(chan error, 1)
			started := time.Now()
			go func() { done <- finishInputWorkerReport(child, Report{}, ErrAuthority, pending.Close) }()
			_, received := readReport(service)
			returned := <-done
			if time.Since(started) != 60*time.Millisecond || errors.Is(received, ErrRecovery) != errors.Is(cause, ErrRecovery) || !errors.Is(returned, ErrAuthority) {
				t.Fatal("final receipt preceded cleanup or changed classification", received, returned)
			}
		})
	}
}

func TestNativePreparationRejectsBeforeConstructor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, run := range []context.Context{nil, ctx} {
		called := false
		p, err := startNativePairPreparation(run, func(context.Context) (*preparedNativePair, error) { called = true; return &preparedNativePair{}, nil })
		if p != nil || err == nil || called {
			t.Fatal("invalid startup launched a child", err)
		}
	}
	if p, err := startNativePairPreparation(context.Background(), nil); p != nil || err == nil {
		t.Fatal("missing constructor accepted")
	}
	// The existing permit parser still rejects foreign/truncated input before
	// this constructor can run in the service-authenticated worker.
	if awaitNativePreparation(bytes.NewReader(nativePreparationFrame[:7])) == nil {
		t.Fatal("truncated service permit accepted")
	}
}
