package inputworker

import (
	"context"
	"errors"
	"io"
	"testing"
)

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
