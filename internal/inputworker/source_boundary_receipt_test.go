package inputworker

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundaryReceiptRetriesOnlyProducerProvenUnsent(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, os.ErrDeadlineExceeded, ErrAuthority, ErrProtocol,
		errors.Join(deferSourceReceipt(sourceBoundaryAck, context.DeadlineExceeded), ErrProtocol)} {
		synctest.Test(t, func(t *testing.T) {
			calls := 0
			err := sendSourceBoundaryReceipt(context.Background(), 42, func(context.Context, uint64) error { calls++; return cause })
			if err != cause || calls != 1 {
				t.Fatal("unproven or sent failure retried", err, calls)
			}
		})
	}
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		var deadline time.Time
		err := sendSourceBoundaryReceipt(context.Background(), 42, func(ctx context.Context, reference uint64) error {
			calls++
			d, _ := ctx.Deadline()
			if calls == 1 {
				deadline = d
			} else if d != deadline {
				t.Fatal("retry renewed receipt budget")
			}
			if reference != 42 {
				t.Fatal("retry changed batch reference")
			}
			if calls == 1 {
				return deferSourceReceipt(sourceBoundaryAck, context.DeadlineExceeded)
			}
			return nil
		})
		if err != nil || calls != 2 {
			t.Fatal(err, calls)
		}
	})
}

func TestBoundaryReceiptRetriesShareFiniteBudgetAndHonorCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		err := sendSourceBoundaryReceipt(context.Background(), 1, func(context.Context, uint64) error {
			return deferSourceReceipt(sourceBoundaryAck, context.DeadlineExceeded)
		})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != IdleTimeout {
			t.Fatal("unbounded deferred receipt", err, time.Since(start))
		}
	})
	synctest.Test(t, func(t *testing.T) {
		owner, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := sendSourceBoundaryReceipt(owner, 1, func(context.Context, uint64) error { calls++; return nil })
		if err != context.Canceled || calls != 0 {
			t.Fatal("cancelled owner submitted receipt", err, calls)
		}
	})
}

func TestBoundaryReceiptFinishesStartedCallBeforeOwnerRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := sendSourceBoundaryReceipt(owner, 1, func(ctx context.Context, _ uint64) error {
			cancel()
			if ctx.Err() != nil {
				t.Fatal("owner cancellation cut an in-flight receipt")
			}
			if d, ok := ctx.Deadline(); !ok || d.Sub(time.Now()) != IdleTimeout {
				t.Fatal("detached receipt lost original deadline")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
