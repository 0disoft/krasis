//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceInputThreadsErrorPreservesCleanupFailureWithParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cleanupErr := errors.New("late hook error")
	err := sourceInputThreadsError(ctx,
		sourceInputThreadResult{component: "low-level-hook", err: cleanupErr},
		sourceInputThreadResult{component: "raw-mouse", err: context.Canceled},
	)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) || !strings.Contains(err.Error(), "low-level-hook") {
		t.Fatalf("sourceInputThreadsError() = %v, want cancellation and cleanup failure", err)
	}
}

func TestSourceInputThreadsErrorPreservesCleanupFailureJoinedWithCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cleanupErr := errors.New("capture unregister failed")
	err := sourceInputThreadsError(ctx,
		sourceInputThreadResult{component: "wheel-capture", err: errors.Join(context.Canceled, cleanupErr)},
		sourceInputThreadResult{component: "raw-mouse", err: fmt.Errorf("raw loop: %w", context.Canceled)},
	)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) || !strings.Contains(err.Error(), "wheel-capture") {
		t.Fatalf("sourceInputThreadsError() = %v, want cancellation and joined cleanup failure", err)
	}
}

func TestCancellationOnlyTraversesWrappedAndJoinedErrors(t *testing.T) {
	if !cancellationOnly(fmt.Errorf("worker stopped: %w", context.Canceled)) {
		t.Fatal("wrapped cancellation was not cancellation-only")
	}
	if !cancellationOnly(errors.Join(context.Canceled, context.DeadlineExceeded)) {
		t.Fatal("joined cancellation causes were not cancellation-only")
	}
	if cancellationOnly(errors.Join(context.Canceled, errors.New("cleanup failed"))) {
		t.Fatal("cleanup failure was classified as cancellation-only")
	}
}

func TestSourceInputThreadsErrorPreservesFailedComponent(t *testing.T) {
	want := errors.New("hook failed")
	err := sourceInputThreadsError(context.Background(),
		sourceInputThreadResult{component: "low-level-hook", err: want},
		sourceInputThreadResult{component: "raw-mouse", err: context.Canceled},
	)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "low-level-hook") {
		t.Fatalf("sourceInputThreadsError() = %v", err)
	}
	if SourceFailureStage(err) != "low-level-hook" {
		t.Fatalf("worker stage lost: %s", SourceFailureStage(err))
	}
}

func TestCollectSourceInputThreadsBoundsHungChild(t *testing.T) {
	ctx, parentCancel := context.WithCancel(context.Background())
	parentCancel()
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan sourceInputThreadResult, 2)
	results <- sourceInputThreadResult{component: "low-level-hook", err: context.Canceled}
	results <- sourceInputThreadResult{component: "raw-mouse", err: context.Canceled}
	err, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, 10*time.Millisecond)
	if joined || !errors.Is(err, context.Canceled) || !errors.Is(err, errSourceInputJoinTimeout) {
		t.Fatalf("collectSourceInputThreads() joined=%t error=%v", joined, err)
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("run context was not canceled: %v", runCtx.Err())
	}
}

func TestCollectSourceInputThreadsJoinsEveryChild(t *testing.T) {
	ctx, parentCancel := context.WithCancel(context.Background())
	parentCancel()
	runCtx, cancel := context.WithCancel(context.Background())
	results := make(chan sourceInputThreadResult, 3)
	for _, component := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
		results <- sourceInputThreadResult{component: component, err: context.Canceled}
	}
	err, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, time.Second)
	if !joined || !errors.Is(err, context.Canceled) || !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("collectSourceInputThreads() joined=%t error=%v", joined, err)
	}
}

func TestCollectSourceThreadsDrainsCompletedResultsAtDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		results := make(chan sourceInputThreadResult, 3)
		for _, component := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
			results <- sourceInputThreadResult{component: component, err: context.Canceled, finished: time.Now()}
		}
		err, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, 0)
		if !joined || !errors.Is(err, context.Canceled) || errors.Is(err, errSourceInputJoinTimeout) {
			t.Fatalf("queued completion treated as live owner: %v %v", joined, err)
		}
	})
}

func TestCollectSourceThreadsRejectsLateCompletionAndNamesMissingOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		results := make(chan sourceInputThreadResult, 3)
		for _, component := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
			results <- sourceInputThreadResult{component: component, err: context.Canceled, finished: time.Now().Add(time.Second)}
		}
		err, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, 0)
		if joined || !errors.Is(err, errSourceInputJoinTimeout) {
			t.Fatal("late completion admitted", joined, err)
		}
		for _, missing := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
			results := make(chan sourceInputThreadResult, 3)
			for _, component := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
				if component != missing {
					results <- sourceInputThreadResult{component: component, err: context.Canceled}
				}
			}
			err, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, 0)
			if joined || SourceFailureStage(err) != "thread-join-"+missing {
				t.Fatal("missing native owner lost", missing, joined, err)
			}
		}
	})
}

func TestSerializedBoundaryObserverPreventsConcurrentDetectorCalls(t *testing.T) {
	next := &concurrencyDetectingBoundaryObserver{}
	observer := &serializedBoundaryObserver{next: next}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for index := 0; index < 8; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			if err := observer.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 0); err != nil {
				t.Errorf("Observe() error = %v", err)
			}
		}()
	}
	close(start)
	workers.Wait()
	if next.concurrent.Load() {
		t.Fatal("boundary observer was called concurrently")
	}
	if got := next.calls.Load(); got != 8 {
		t.Fatalf("boundary calls = %d, want 8", got)
	}
}

type concurrencyDetectingBoundaryObserver struct {
	active     atomic.Int32
	concurrent atomic.Bool
	calls      atomic.Uint64
}

func (observer *concurrencyDetectingBoundaryObserver) Observe(control.InputAction, int32) error {
	if observer.active.Add(1) != 1 {
		observer.concurrent.Store(true)
	}
	time.Sleep(time.Millisecond)
	observer.calls.Add(1)
	observer.active.Add(-1)
	return nil
}
