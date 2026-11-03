package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

type nativeProofResult struct {
	err     error
	elapsed time.Duration
}

type nativeProofStarter func(context.Context) (<-chan nativeProofResult, func(bool))

var errNativeProofExited = errors.Join(ErrAuthority, inputdesktop.ErrCallbackExit)

// One owner per native child reuses its stack and scheduling state. Every task
// still performs a fresh check. A reply slot is reusable only after its result
// was consumed; an unjoined timeout permanently fences it against late replies.
type nativeProofWorker struct {
	ctx    context.Context
	cancel context.CancelFunc
	work   chan context.Context
	reply  chan nativeProofResult
	done   chan struct{}
	active atomic.Bool
	fenced atomic.Bool
	finish func(bool)
}

func newNativeProofWorker(ctx context.Context, service func() error) *nativeProofWorker {
	run, cancel := context.WithCancel(ctx)
	w := &nativeProofWorker{ctx: run, cancel: cancel, work: make(chan context.Context, 1), reply: make(chan nativeProofResult, 1), done: make(chan struct{})}
	w.finish = func(joined bool) {
		if !joined {
			w.fenced.Store(true)
		} else {
			w.active.Store(false)
		}
	}
	go func() {
		defer close(w.done)
		for {
			select {
			case <-run.Done():
				return
			case task := <-w.work:
				if err := errors.Join(run.Err(), task.Err()); err != nil {
					w.reply <- nativeProofResult{err: err}
					continue
				}
				runNativeProof(service, w.reply)
			}
		}
	}()
	return w
}

func (w *nativeProofWorker) start(ctx context.Context) (<-chan nativeProofResult, func(bool)) {
	deny := func() (<-chan nativeProofResult, func(bool)) {
		reply := make(chan nativeProofResult, 1)
		reply <- nativeProofResult{err: errors.Join(ErrAuthority, errNativeProofPending)}
		return reply, nil
	}
	if ctx == nil || ctx.Err() != nil || w.ctx.Err() != nil || w.fenced.Load() {
		return deny()
	}
	select {
	case <-w.done:
		return deny()
	default:
	}
	if !w.active.CompareAndSwap(false, true) {
		return deny()
	}
	select {
	case w.work <- ctx:
		return w.reply, w.finish
	default:
		// Framed input is serial. Never accumulate speculative proof jobs or
		// block admission outside the executor's original reply deadline.
		w.fenced.Store(true)
		return deny()
	}
}

func runNativeProof(service func() error, reply chan<- nativeProofResult) {
	started := time.Now()
	result := nativeProofResult{err: errNativeProofExited}
	defer func() {
		if recover() != nil {
			result.err = errors.Join(ErrAuthority, inputdesktop.ErrCallbackPanic)
		}
		result.elapsed = time.Since(started)
		reply <- result
	}()
	result.err = service()
}

// A native query cannot be cancelled midway. Join its exact completion before
// the service Authority closes pinned handles. Parent job retirement is unchanged.
func (w *nativeProofWorker) close() {
	w.cancel()
	<-w.done
}
