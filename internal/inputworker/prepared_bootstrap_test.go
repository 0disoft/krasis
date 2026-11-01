package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"
)

func TestPreparedLeaseBootstrapSeparatesIdleFromConstructionAndBinding(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		initial, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()
		idle, cancelIdle := context.WithTimeout(context.Background(), preparedConsoleWindow+startupTimeout)
		defer cancelIdle()
		started := time.Now()
		var payload [bootstrapSize]byte
		for i := range payload {
			payload[i] = byte(i + 1)
		}
		stream := bytes.NewReader(payload[:])
		calls := 0
		var binding context.Context
		var cancelBinding context.CancelFunc
		defer func() {
			if cancelBinding != nil {
				cancelBinding()
			}
		}()
		got, err := readPreparedLeaseBootstrap(initial, idle, func() bool { return true }, func(ctx context.Context, data []byte) (int, error) {
			calls++
			if calls == 1 {
				if ctx != idle || len(data) != 1 {
					t.Fatal("idle phase consumed binding data")
				}
				time.Sleep(startupTimeout + time.Second)
			} else if ctx != binding {
				t.Fatal("binding reused idle or expired startup budget")
			}
			return stream.Read(data)
		}, func() (context.Context, error) {
			binding, cancelBinding = context.WithTimeout(context.Background(), startupTimeout)
			return binding, nil
		})
		deadline, ok := binding.Deadline()
		if err != nil || got != payload || calls != 2 || !errors.Is(initial.Err(), context.DeadlineExceeded) || !ok || deadline.Sub(started) != 2*startupTimeout+time.Second {
			t.Fatal("idle reservation consumed or extended real binding budget", err, calls, deadline.Sub(started))
		}
	})
}

func TestPreparedLeaseBootstrapFencesExpiredIdleEvenAfterCompletedRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		idle, cancel := context.WithTimeout(context.Background(), preparedConsoleWindow)
		defer cancel()
		_, err := readPreparedLeaseBootstrap(context.Background(), idle, func() bool { return true }, func(_ context.Context, data []byte) (int, error) {
			time.Sleep(preparedConsoleWindow)
			data[0] = 1
			return 1, nil
		}, func() (context.Context, error) { t.Fatal("expired idle started binding"); return nil, nil })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("expired first byte admitted", err)
		}
	})
}

func TestPreparedLeaseBootstrapPartialPacketKeepsFiveSecondLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		idle, cancelIdle := context.WithTimeout(context.Background(), preparedConsoleWindow+startupTimeout)
		defer cancelIdle()
		started := time.Now()
		var cancel context.CancelFunc
		defer func() {
			if cancel != nil {
				cancel()
			}
		}()
		calls := 0
		_, err := readPreparedLeaseBootstrap(context.Background(), idle, func() bool { return true }, func(ctx context.Context, data []byte) (int, error) {
			calls++
			if calls == 1 {
				data[0] = 1
				return 1, nil
			}
			<-ctx.Done()
			return 0, ctx.Err()
		}, func() (context.Context, error) {
			var binding context.Context
			binding, cancel = context.WithTimeout(context.Background(), startupTimeout)
			return binding, nil
		})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != startupTimeout || idle.Err() != nil {
			t.Fatal("partial bootstrap inherited idle budget", err, time.Since(started))
		}
	})
}

func TestPreparedLeaseBootstrapRejectsLostOwnershipAndBindingFailure(t *testing.T) {
	for _, owned := range []bool{false, true} {
		fault := errors.New("deadline setup failed")
		reads, begins := 0, 0
		_, err := readPreparedLeaseBootstrap(context.Background(), context.Background(), func() bool { return owned }, func(_ context.Context, data []byte) (int, error) {
			reads++
			data[0] = 1
			return 1, nil
		}, func() (context.Context, error) { begins++; return nil, fault })
		if !errors.Is(err, ErrAuthority) || !owned && (reads != 0 || begins != 0) || owned && (reads != 1 || begins != 1 || !errors.Is(err, fault)) {
			t.Fatal("ownership or binding failure admitted", owned, reads, begins, err)
		}
	}
}

func TestPreparedBootstrapRejectsCancelledOrUnownedReads(t *testing.T) {
	for _, callbackRunning := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reads := 0
		_, err := readPreparedBootstrap(ctx, func() bool { return !callbackRunning }, func(context.Context, []byte) (int, error) { reads++; return 0, nil })
		if !errors.Is(err, ErrAuthority) || !errors.Is(err, context.Canceled) || reads != 0 {
			t.Fatal("expired startup admitted a new read", err, reads)
		}
	}
	_, err := readPreparedBootstrap(context.Background(), func() bool { return false }, func(context.Context, []byte) (int, error) { t.Fatal("unowned read"); return 0, nil })
	if !errors.Is(err, ErrAuthority) {
		t.Fatal("running close callback did not fence bootstrap", err)
	}
}

func TestPreparedBootstrapCannotAcceptPartialStop(t *testing.T) {
	calls := 0
	_, err := readPreparedBootstrap(context.Background(), func() bool { return true }, func(_ context.Context, data []byte) (int, error) {
		calls++
		if calls == 1 {
			return len(data) - 1, nil
		}
		return 0, context.Canceled
	})
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatal("partial bootstrap cancellation was accepted", err, calls)
	}
	_, err = readPreparedBootstrap(context.Background(), func() bool { return true }, func(context.Context, []byte) (int, error) { return 0, io.EOF })
	if !errors.Is(err, io.EOF) {
		t.Fatal("missing bootstrap was accepted", err)
	}
}
