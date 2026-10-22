//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// At most one unbound input child prepares alongside one acknowledged passive
// owner. Joining transfers its ownership back to the serial service loop.
type cursorInputPreparation struct {
	done   chan struct{}
	next   *preparedConsole
	err    error
	joined atomic.Bool
	cancel context.CancelFunc
}

// A clean input return is a single preparation opportunity, independent of the
// rate-limited movement hints. No lease or agent handle enters the warm child.
// Construction is joined before admission, after the next agent authenticates.
func prepareInputReentry(ctx context.Context, lease Lease, result error, prepare func(context.Context) (*preparedConsole, error)) *cursorInputPreparation {
	if ctx == nil || ctx.Err() != nil || result != nil || lease.Capture || lease.Presentation || !lease.ExpiresAt.After(time.Now()) {
		return nil
	}
	return startCursorInputPreparation(ctx, prepare)
}

func startCursorInputPreparation(ctx context.Context, prepare func(context.Context) (*preparedConsole, error)) *cursorInputPreparation {
	lifetime, stop := context.WithCancel(ctx)
	p := &cursorInputPreparation{done: make(chan struct{}), cancel: stop}
	go func() {
		defer close(p.done)
		defer stop()
		run, cancel := context.WithTimeout(lifetime, startupTimeout)
		defer cancel()
		p.next, p.err = prepare(run)
		if run.Err() != nil {
			p.err = errors.Join(p.err, run.Err())
		}
		if p.err != nil && p.next != nil {
			if p.next.close() != nil {
				p.err = errors.Join(p.err, ErrRecovery)
			}
			p.next = nil
		}
	}()
	return p
}

func (p *cursorInputPreparation) join() (*preparedConsole, error) {
	if p == nil {
		return nil, nil
	}
	<-p.done
	if !p.joined.CompareAndSwap(false, true) {
		return nil, p.err
	}
	return p.next, p.err
}
