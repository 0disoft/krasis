package residentagent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputworker"
)

type protectedReturnPresentation struct {
	mu         sync.Mutex
	generation uint64
	cancel     context.CancelFunc
	done       chan struct{}
	open       func(context.Context, inputworker.Lease) (func() error, error)
	root       context.Context
}

func (input *privilegedLeaseInput) PrepareReturnCursor() func() {
	p := &input.cursor
	if p.open == nil || p.root == nil {
		return nil
	}
	p.mu.Lock()
	generation := p.generation
	p.mu.Unlock()
	input.mu.Lock()
	client, identity, expires := input.client, input.identity, input.leaseExpires
	port, ok := client.(application.CursorPositionPort)
	if input.stopped || !ok {
		input.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithTimeout(p.root, 100*time.Millisecond)
	x, y, err := port.CursorPosition(ctx)
	cancel()
	input.mu.Unlock()
	if err != nil {
		return nil
	}
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.generation != generation || p.cancel != nil || p.root.Err() != nil {
			return
		}
		run, cancel := context.WithCancel(p.root)
		done := make(chan struct{})
		p.cancel, p.done = cancel, done
		go func() {
			defer close(done)
			binding, err := inputworker.NewBinding()
			if err != nil {
				return
			}
			limit := time.Now().Add(60 * time.Second)
			if expires.Before(limit) {
				limit = expires
			}
			lease := inputworker.Lease{Identity: identity, Binding: binding, ExpiresAt: limit, Presentation: true, CursorX: x, CursorY: y}
			start, stop := context.WithTimeout(run, 750*time.Millisecond)
			openedAt := time.Now()
			closeChannel, err := p.open(start, lease)
			stop()
			input.observeHandoffTiming("cursor_open", openedAt)
			if err != nil || closeChannel == nil {
				if input.event != nil {
					input.event("installed_return_cursor_failed")
				}
				return
			}
			if input.event != nil {
				input.event("installed_return_cursor_ready")
			}
			<-run.Done()
			if err := closeChannel(); err != nil {
				input.mu.Lock()
				input.closeErr = errors.Join(input.closeErr, err)
				input.mu.Unlock()
				if input.event != nil {
					input.event("installed_return_cursor_close_failed")
				}
			}
		}()
	}
}

func (input *privilegedLeaseInput) RestoreReturnCursor() {
	started := time.Now()
	p := &input.cursor
	p.mu.Lock()
	p.generation++
	cancel, done := p.cancel, p.done
	// Keep a retiring owner fenced until its open/channel close has returned.
	if cancel != nil {
		cancel()
	}
	p.mu.Unlock()
	if done != nil {
		<-done
	}
	p.mu.Lock()
	if p.done == done {
		p.cancel, p.done = nil, nil
	}
	p.mu.Unlock()
	if cancel != nil || done != nil {
		input.observeHandoffTiming("cursor_retire", started)
	}
}

// One process-local monotonic duration per attempted admission/open/retirement.
// These independent observations survive the bounded detailed-profile window;
// admission includes retirement and must not be added to it.
func (input *privilegedLeaseInput) observeHandoffTiming(stage string, started time.Time) {
	if input.event == nil {
		return
	}
	if stage != "admission" && stage != "cursor_open" && stage != "cursor_retire" {
		return
	}
	elapsed, bucket := time.Since(started), "ge1s"
	switch {
	case elapsed < time.Millisecond:
		bucket = "lt1ms"
	case elapsed < 10*time.Millisecond:
		bucket = "lt10ms"
	case elapsed < 100*time.Millisecond:
		bucket = "lt100ms"
	case elapsed < time.Second:
		bucket = "lt1s"
	}
	input.event("installed_input_handoff_timing_" + stage + "_" + bucket)
}
