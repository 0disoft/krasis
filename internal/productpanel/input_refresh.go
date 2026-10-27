package productpanel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInputRefreshCancelled = errors.New("input installation consent cancelled")
	ErrInputRefreshFailed    = errors.New("input installation failed")
	ErrInputRefreshPending   = errors.New("input installation outcome pending")
)

// The runner presents Windows consent; the embedded installer owns all
// privileged writes, fixed paths, validation and rollback.
type inputRefresh struct {
	mu         sync.Mutex
	reader     atomic.Pointer[inputHealthReader]
	attempted  bool
	checkedAt  time.Time
	state      atomic.Pointer[string]
	lastResult atomic.Pointer[string]
	inspect    func() string
	run        func(context.Context) error
	repair     func(context.Context) error
}

type inputHealthReader struct {
	done  chan struct{}
	state string // Published before done closes; belongs to this observation.
}

// Caller transfers mu to the single read-only worker. A cancelled waiter cannot
// spawn another worker or permit installation while this observation is running.
func (b *inputRefresh) startHealthReaderLocked() *inputHealthReader {
	b.checkedAt = time.Now()
	reader := &inputHealthReader{done: make(chan struct{})}
	b.reader.Store(reader)
	go func() {
		reader.state = b.inspectLocked()
		b.reader.CompareAndSwap(reader, nil)
		b.mu.Unlock()
		close(reader.done)
	}()
	return reader
}

func (b *inputRefresh) status() string {
	state := b.state.Load()
	if b.mu.TryLock() {
		current := b.state.Load()
		interval := 30 * time.Second
		if current != nil && *current == "PENDING" {
			interval = time.Second
		}
		eligible := current == nil || *current == "PENDING" || *current == "READY" || *current == "UNKNOWN" || *current == "SERVICE_STOPPED" || *current == "SERVICE_RECOVERY_REQUIRED" || *current == "NOT_INSTALLED" || *current == "UPDATE_REQUIRED" || b.lastResult.Load() != nil && (*current == "CANCELLED" || *current == "FAILED")
		if eligible && (current == nil || time.Since(b.checkedAt) >= interval) {
			b.startHealthReaderLocked()
		} else {
			b.mu.Unlock()
		}
	}
	if state != nil {
		return *state // Initial and pending reads never block the renderer.
	}
	return "UNKNOWN"
}

// Native startup needs a completed observation before deciding whether to
// pause an existing session. Waiting for a read never repeats installation.
func (b *inputRefresh) probe(ctx context.Context) string {
	if state, err := b.readHealth(ctx); err == nil {
		return state
	}
	if state := b.state.Load(); state != nil {
		return *state
	}
	return "UNKNOWN"
}

func (b *inputRefresh) inspectLocked() string {
	if state := b.state.Load(); state != nil && *state == "PENDING" {
		b.attempted = true
	}
	state := b.inspect()
	if state == "PENDING" {
		b.attempted = true
	}
	b.setState(state)
	if b.attemptResult() == "input_refresh_pending" {
		if state == "READY" {
			b.setAttemptResult("ok")
		}
		if state == "UPDATE_REQUIRED" || state == "NOT_INSTALLED" {
			b.setAttemptResult("input_refresh_failed")
		}
	}
	return state
}

// Health observations may change; the last attempted operation is independent.
func (b *inputRefresh) attemptResult() string {
	if value := b.lastResult.Load(); value != nil {
		return *value
	}
	return ""
}

func (b *inputRefresh) setAttemptResult(code string) { b.lastResult.Store(&code) }

// Automatic preparation may await a read-only health worker. An installer or
// explicit request still returns busy immediately, preserving ownership fences.
func (b *inputRefresh) lock(ctx context.Context, explicit bool) bool {
	if ctx.Err() != nil {
		return false
	}
	for !b.mu.TryLock() {
		if explicit {
			return false
		}
		reader := b.reader.Load()
		if reader == nil {
			return b.mu.TryLock() // The reader may just have released the gate.
		}
		select {
		case <-ctx.Done():
			return false
		case <-reader.done:
		}
	}
	return true
}

// Startup and restoration share the renderer's read-only worker. A canceled
// waiter leaves the native inspection and installation gate owned until the
// reader actually returns; no replacement inspection or installer may race it.
func (b *inputRefresh) readHealth(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var reader *inputHealthReader
	if b.mu.TryLock() {
		reader = b.startHealthReaderLocked()
	} else {
		reader = b.reader.Load()
		if reader == nil {
			return "", ErrInputRefreshPending
		}
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-reader.done:
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return reader.state, nil
}

// An explicit read requests fresh health without installation. Concurrent
// requests coalesce into the reader already holding the installation gate.
func (b *inputRefresh) recheck() string {
	if b.mu.TryLock() {
		if state := b.state.Load(); state == nil || *state != "UPDATING" {
			if state != nil && (*state == "FAILED" || *state == "CANCELLED") {
				b.setState("UNKNOWN") // Maintenance outcome is retained separately by lifecycle.
			}
			b.checkedAt = time.Time{}
		}
		b.mu.Unlock()
	}
	return b.status()
}

// Caller holds mu; checkedAt never races with a status/installation observer.
func (b *inputRefresh) setState(state string) {
	b.checkedAt = time.Now()
	b.state.Store(&state)
}

func (b *inputRefresh) refreshPendingLocked() {
	if state := b.state.Load(); state != nil && *state == "PENDING" {
		// Observe completion; never automatically start a second installer.
		b.inspectLocked()
	}
}

func (b *inputRefresh) automatic(ctx context.Context) error { return b.apply(ctx, false) }
func (b *inputRefresh) explicit(ctx context.Context) error  { return b.apply(ctx, true) }

// Read fresh health under the installation gate; never repeats consent.
func (b *inputRefresh) restoreCheck(ctx context.Context) error {
	state, err := b.readHealth(ctx)
	if err != nil {
		return err
	}
	if state != "READY" && state != "UPDATE_REQUIRED" && state != "NOT_INSTALLED" {
		return ErrInputRefreshPending
	}
	return nil
}

func (b *inputRefresh) apply(ctx context.Context, explicit bool) error {
	if !b.lock(ctx, explicit) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrStartupBusy
	}
	defer b.mu.Unlock()
	b.refreshPendingLocked()
	if state := b.state.Load(); state != nil && *state == "PENDING" {
		return ErrInputRefreshPending
	}
	if !explicit && b.attempted {
		switch b.attemptResult() {
		case "input_refresh_cancelled":
			return ErrInputRefreshCancelled
		case "input_refresh_failed":
			return ErrInputRefreshFailed
		case "input_refresh_pending":
			return ErrInputRefreshPending
		}
		return nil
	}
	state := b.inspect()
	if state == "PENDING" {
		b.attempted = true
		b.setState(state)
		return ErrInputRefreshPending
	}
	if state == "READY" || !explicit && state != "UPDATE_REQUIRED" {
		b.setState(state)
		return nil
	}
	repair := explicit && (state == "SERVICE_STOPPED" || state == "SERVICE_RECOVERY_REQUIRED")
	if state != "UPDATE_REQUIRED" && state != "NOT_INSTALLED" && !repair {
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
	b.lastResult.Store(nil)
	b.setState("UPDATING")
	runner := b.run
	if repair {
		if b.repair == nil {
			b.setState("FAILED")
			b.setAttemptResult("input_refresh_failed")
			return ErrInputRefreshFailed
		}
		runner = b.repair
	}
	err := runner(ctx)
	switch {
	case errors.Is(err, ErrInputRefreshPending):
		b.setState("PENDING")
		b.setAttemptResult("input_refresh_pending")
		return err
	case errors.Is(err, ErrInputRefreshCancelled):
		b.setState("CANCELLED")
		b.setAttemptResult("input_refresh_cancelled")
	case err != nil:
		b.setState("FAILED")
		b.setAttemptResult("input_refresh_failed")
		if !errors.Is(err, ErrInputRefreshFailed) {
			err = errors.Join(ErrInputRefreshFailed, err)
		}
	case b.inspect() != "READY":
		b.setState("FAILED")
		b.setAttemptResult("input_refresh_failed")
		err = ErrInputRefreshFailed
	default:
		b.setState("READY")
		b.setAttemptResult("ok")
	}
	// Preserve the installer result. Startup may restore ordinary input without
	// treating this result as successful or repeating the consent prompt.
	return err
}
