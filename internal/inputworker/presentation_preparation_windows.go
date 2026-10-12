//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
)

// At most one unbound input child prepares alongside one acknowledged passive
// owner. Joining transfers its ownership back to the serial service loop.
type cursorInputPreparation struct {
	done   chan struct{}
	next   *preparedConsole
	err    error
	joined atomic.Bool
}

func startCursorInputPreparation(ctx context.Context, prepare func(context.Context) (*preparedConsole, error)) *cursorInputPreparation {
	p := &cursorInputPreparation{done: make(chan struct{})}
	go func() {
		defer close(p.done)
		run, cancel := context.WithTimeout(ctx, startupTimeout)
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
