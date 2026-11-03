package inputworker

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestNativeProofWorkerUsesFreshChecksAndRejectsRevocation(t *testing.T) {
	var checks atomic.Int32
	w := newNativeProofWorker(context.Background(), func() error {
		if checks.Add(1) == 3 {
			return ErrAuthority
		}
		return nil
	})
	defer w.close()
	for i := 1; i <= 3; i++ {
		result := takeNativeProof(w, context.Background())
		if (result.err != nil) != (i == 3) || checks.Load() != int32(i) {
			t.Fatal("proof cached or revocation lost", i, checks.Load(), result.err)
		}
	}
}

func TestNativeProofWorkerRetainsLateQueryAndFencesInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release, entered := make(chan struct{}), make(chan struct{})
		w := newNativeProofWorker(context.Background(), func() error { close(entered); <-release; return nil })
		execute := nativeInputExecutorWithProof(func(_ context.Context, work func() error) error { return work() },
			func() error { return nil }, func() bool { return true }, time.Now().Add(time.Hour), false, w.start)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		if err := execute(ctx, func() error { t.Fatal("unconfirmed proof applied input"); return nil }); !errors.Is(err, errNativeProofPending) {
			t.Fatal("pending proof lost its negative reply", err)
		}
		<-entered
		closed := make(chan struct{})
		go func() { w.close(); close(closed) }()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("authority could close before native query returned")
		default:
		}
		close(release)
		<-closed
		if err := execute(context.Background(), func() error { t.Fatal("late proof revived input"); return nil }); err == nil {
			t.Fatal("timed-out dispatcher was reused")
		}
	})
}

func TestNativeProofWorkerBoundsQueueAndSkipsCancelledWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		w := newNativeProofWorker(context.Background(), func() error { calls.Add(1); close(entered); <-release; return nil })
		first, finish := w.start(context.Background())
		<-entered
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if result := takeNativeProof(w, ctx); result.err == nil {
			t.Fatal("cancelled proof admitted")
		}
		if result := takeNativeProof(w, context.Background()); !errors.Is(result.err, errNativeProofPending) {
			t.Fatal("concurrent proof admitted")
		}
		close(release)
		if result := <-first; result.err != nil {
			t.Fatal(result.err)
		}
		finish(true)
		w.close()
		if calls.Load() != 1 {
			t.Fatal("cancelled proof reached native I/O")
		}
		if result := takeNativeProof(w, context.Background()); result.err == nil {
			t.Fatal("closed worker accepted work")
		}
	})
}

func TestNativeProofWorkerContainsPanicAndGoexit(t *testing.T) {
	for _, exit := range []bool{false, true} {
		w := newNativeProofWorker(context.Background(), func() error {
			if exit {
				runtime.Goexit()
			}
			panic("private data")
		})
		result := takeNativeProof(w, context.Background())
		want := inputdesktop.ErrCallbackPanic
		if exit {
			want = inputdesktop.ErrCallbackExit
		}
		if !errors.Is(result.err, ErrAuthority) || !errors.Is(result.err, want) {
			t.Fatal("failed proof became success", result.err)
		}
		w.close()
	}
}

func TestNativeProofGoexitCannotBecomeSuccessfulInput(t *testing.T) {
	execute := nativeInputExecutor(func(_ context.Context, work func() error) error { return work() },
		func() error { runtime.Goexit(); return nil }, func() bool { return true }, time.Now().Add(time.Minute), false)
	err := execute(context.Background(), func() error { t.Fatal("abandoned proof admitted input"); return nil })
	if !errors.Is(err, inputdesktop.ErrCallbackExit) || !errors.Is(err, ErrAuthority) {
		t.Fatal("abandoned proof became success", err)
	}
}

func TestNativeProofLateReplyCannotEnterNextRequest(t *testing.T) {
	release, entered := make(chan struct{}), make(chan struct{})
	w := newNativeProofWorker(context.Background(), func() error { close(entered); <-release; return nil })
	defer w.close()
	reply, finish := w.start(context.Background())
	<-entered
	finish(false) // The caller returned without consuming this exact reply.
	close(release)
	if late := <-reply; late.err != nil {
		t.Fatal(late.err)
	}
	if next := takeNativeProof(w, context.Background()); !errors.Is(next.err, errNativeProofPending) {
		t.Fatal("late success was reused", next.err)
	}
}

func BenchmarkNativeProofDispatch(b *testing.B) {
	for _, reused := range []bool{false, true} {
		name := "spawned"
		if reused {
			name = "reused"
		}
		b.Run(name, func(b *testing.B) {
			check := func() error { return nil }
			start := nativeProofStarter(func(context.Context) (<-chan nativeProofResult, func(bool)) {
				reply := make(chan nativeProofResult, 1)
				go runNativeProof(check, reply)
				return reply, nil
			})
			if reused {
				w := newNativeProofWorker(context.Background(), check)
				defer w.close()
				start = w.start
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reply, finish := start(context.Background())
				result := <-reply
				if finish != nil {
					finish(true)
				}
				if result.err != nil {
					b.Fatal(result.err)
				}
			}
		})
	}
}

func takeNativeProof(w *nativeProofWorker, ctx context.Context) nativeProofResult {
	reply, finish := w.start(ctx)
	result := <-reply
	if finish != nil {
		finish(true)
	}
	return result
}
