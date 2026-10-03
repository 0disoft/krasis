package productpanel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	ErrInputRefreshCancelled = errors.New("input installation consent cancelled")
	ErrInputRefreshFailed    = errors.New("input installation failed")
	ErrInputRefreshPending   = errors.New("input installation outcome pending")
)

// The runner presents Windows consent; the embedded installer owns all
// privileged writes, fixed paths, validation and rollback.
type inputRefresh struct {
	mu        sync.Mutex
	attempted bool
	state     atomic.Pointer[string]
	inspect   func() string
	run       func(context.Context) error
}

func (b *inputRefresh) status() string {
	if state := b.state.Load(); state != nil {
		return *state
	}
	state := b.inspect()
	b.state.CompareAndSwap(nil, &state)
	return *b.state.Load()
}

func (b *inputRefresh) setState(state string) { b.state.Store(&state) }

func (b *inputRefresh) automatic(ctx context.Context) error { return b.apply(ctx, false) }
func (b *inputRefresh) explicit(ctx context.Context) error  { return b.apply(ctx, true) }

func (b *inputRefresh) apply(ctx context.Context, explicit bool) error {
	if !b.mu.TryLock() {
		return ErrStartupBusy
	}
	defer b.mu.Unlock()
	if state := b.state.Load(); state != nil && *state == "PENDING" {
		return ErrInputRefreshPending
	}
	if !explicit && b.attempted {
		return nil
	}
	state := b.inspect()
	if state == "PENDING" {
		b.setState(state)
		return ErrInputRefreshPending
	}
	if state == "READY" || !explicit && state != "UPDATE_REQUIRED" {
		b.setState(state)
		return nil
	}
	if state != "UPDATE_REQUIRED" && state != "NOT_INSTALLED" {
		b.setState("FAILED")
		if explicit {
			return ErrInputRefreshFailed
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.attempted = true
	b.setState("UPDATING")
	err := b.run(ctx)
	switch {
	case errors.Is(err, ErrInputRefreshPending):
		b.setState("PENDING")
		return err
	case errors.Is(err, ErrInputRefreshCancelled):
		b.setState("CANCELLED")
	case err != nil:
		b.setState("FAILED")
	case b.inspect() != "READY":
		b.setState("FAILED")
		err = ErrInputRefreshFailed
	default:
		b.setState("READY")
	}
	// Cancellation/failure preserves ordinary input. Recovery and polling do
	// not repeat the consent prompt; retry requires a local button click.
	if explicit {
		return err
	}
	return nil
}
