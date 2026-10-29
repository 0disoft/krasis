//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Input admission and the preparation-only endpoint share exactly one slot.
// The mutex protects ownership, not authentication or an active input lease.
type serviceInputPreparation struct {
	mu      sync.Mutex
	next    *preparedConsole
	warming *cursorInputPreparation
	last    time.Time
	failure error // Uncertain retirement is terminal, including across endpoints.
	reentry *reentryPreparation
}

func (p *serviceInputPreparation) reenter(ctx context.Context, lease Lease, result error, prepare func(context.Context) (*preparedConsole, error)) error {
	if !inputReentryAllowed(ctx, lease, result) {
		return nil
	}
	// A long active lease can outlive the one speculative candidate. Harvest
	// only completed work, then replace an expired candidate through start's
	// existing retirement fence. Never restart periodically while input runs.
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.collectReadyLocked(); err != nil {
		return err
	}
	p.startLocked(ctx, prepare)
	p.armReentryLocked(ctx, lease.ExpiresAt, prepare)
	return nil
}

func (p *serviceInputPreparation) start(ctx context.Context, prepare func(context.Context) (*preparedConsole, error)) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startLocked(ctx, prepare)
}

func (p *serviceInputPreparation) startLocked(ctx context.Context, prepare func(context.Context) (*preparedConsole, error)) bool {
	if p.failure != nil || ctx == nil || ctx.Err() != nil || p.warming != nil || p.next != nil && !p.next.retired() {
		return false
	}
	retiring := p.next
	p.next = nil
	p.warming = startCursorInputPreparation(ctx, func(run context.Context) (*preparedConsole, error) {
		if err := retiring.close(); err != nil {
			return nil, errors.Join(ErrRecovery, err)
		}
		return prepare(run)
	})
	p.last = time.Now()
	return true
}

// Called only after an authenticated hint's receipt and channel close. The
// receipt acknowledges demand, not readiness. Keep one bounded builder, and
// retain the existing cooldown/expiry policy without tying work to an ACK wait.
func (p *serviceInputPreparation) hint(ctx context.Context, last, now time.Time, prepare func(context.Context) (*preparedConsole, error)) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last.After(last) {
		last = p.last
	}
	if p.collectReadyLocked() != nil {
		return false
	}
	if ctx == nil || ctx.Err() != nil || p.warming != nil || !preparationAllowed(last, now) {
		return false
	}
	if p.next != nil && !p.next.retired() && !preparationRefreshDue(last, p.next.preparedAt, now) {
		return false
	}
	retiring := p.next
	p.next = nil
	p.cancelReentryLocked() // Fresh physical demand supersedes the delayed opportunity.
	return p.startLocked(ctx, func(run context.Context) (*preparedConsole, error) {
		if err := retiring.close(); err != nil {
			return nil, errors.Join(ErrRecovery, err)
		}
		return prepare(run)
	})
}

func (p *serviceInputPreparation) join() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.joinLocked()
}

func (p *serviceInputPreparation) joinLocked() error {
	if p.warming == nil {
		return p.failure
	}
	var err error
	p.next, err = p.warming.join()
	p.warming = nil
	// Ordinary construction failure still permits the existing cold fallback.
	p.failure = errors.Join(p.failure, serviceRecoveryFailure(err, nil))
	return p.failure
}

// Harvest only a completed builder. Neither optional demand nor authentication
// may wait for native construction, but completed errors must fence admission.
func (p *serviceInputPreparation) collectReady() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.collectReadyLocked()
}

func (p *serviceInputPreparation) collectReadyLocked() error {
	if p.failure != nil {
		return p.failure
	}
	if p.warming != nil {
		select {
		case <-p.warming.done:
			return p.joinLocked()
		default:
		}
	}
	return nil
}

func (p *serviceInputPreparation) accept(accept func() (*Channel, error)) (*Channel, error) {
	p.mu.Lock()
	hadPreparation := p.warming != nil
	err := p.collectReadyLocked()
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	channel, err := accept()
	if err != nil {
		return channel, err
	}
	// Accept can sleep past both construction and expiry. Reap before decoding
	// the next hint, or warming remains occupied and drops the renewal demand.
	if err := p.collectReady(); err != nil {
		return nil, serviceRecoveryFailure(err, channel.Close())
	}
	// A builder collected before authentication has zero remaining join wait.
	channel.preparationWaitMeasured = hadPreparation
	return channel, nil
}

// Decode the authenticated request before joining: another optional hint must
// not block behind construction. Every actual lease still joins before login
// admission, candidate transfer or native input, including presentation leases.
func (p *serviceInputPreparation) admit(channel *Channel) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.admitLocked(channel)
}

func (p *serviceInputPreparation) admitLocked(channel *Channel) error {
	if p.failure != nil {
		return serviceRecoveryFailure(p.failure, channel.Close())
	}
	if p.warming != nil {
		started := time.Now()
		err := p.joinLocked()
		channel.preparationWait += time.Since(started)
		channel.preparationWaitMeasured = true
		if err != nil {
			return serviceRecoveryFailure(err, channel.Close())
		}
	}
	return nil
}

func (p *serviceInputPreparation) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelReentryLocked()
	if p.warming != nil {
		p.warming.cancel()
	}
	err := p.joinLocked()
	closeErr := p.next.close()
	p.next = nil
	p.failure = serviceRecoveryFailure(err, closeErr)
	return p.failure
}

func (p *serviceInputPreparation) take(channel *Channel, lease Lease) (*preparedConsole, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !lease.Presentation {
		p.cancelReentryLocked()
	}
	// A demand can arrive during login admission. Join it and transfer under
	// one lock; a nil candidate must not bypass its old owner's retirement.
	if err := p.admitLocked(channel); err != nil {
		return nil, err
	}
	return takeInputPreparation(&p.next, lease), nil
}
