package inputworker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceActionWindowExactReceiptsAndWrap(t *testing.T) {
	w := newSourceActionWindow()
	for round := uint64(0); round < 4; round++ {
		for i := uint64(1); i <= sourceActionWindowLimit; i++ {
			if err := w.acquire(context.Background()); err != nil {
				t.Fatal(err)
			}
			w.append(round*100 + i*2) // Replies may occupy intervening wire sequences.
		}
		for _, bad := range []uint64{0, round*100 + 14, round*100 + 17, round*100 + 32} {
			if !errors.Is(w.acknowledge(bad), ErrProtocol) || len(w.credit) != 0 {
				t.Fatal("invalid receipt released input", bad)
			}
		}
		first := round*100 + 16
		if w.acknowledge(first) != nil || len(w.credit) != 8 {
			t.Fatal("first group lost")
		}
		if w.acknowledge(first) != ErrProtocol || len(w.credit) != 8 {
			t.Fatal("duplicate credit")
		}
		if w.acknowledge(round*100+32) != nil || len(w.credit) != 16 {
			t.Fatal("second group lost")
		}
	}
}

func TestSourceActionWindowCancellationAndRollback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newSourceActionWindow()
		for i := uint64(1); i <= sourceActionWindowLimit; i++ {
			_ = w.acquire(context.Background())
			w.append(i)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		if !errors.Is(w.acquire(ctx), context.DeadlineExceeded) || len(w.credit) != 0 {
			t.Fatal("waiter leaked credit")
		}
		if !w.rollback(16) || w.rollback(16) {
			t.Fatal("rollback consumed another frame")
		}
		w.credit <- struct{}{}
		_ = w.acquire(context.Background())
		w.append(17)
		if w.acknowledge(8) != nil || w.acknowledge(17) != nil || len(w.credit) != 16 || w.rollback(17) {
			t.Fatal("rollback lost subsequent group")
		}
	})
}
