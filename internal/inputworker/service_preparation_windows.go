//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"
)

// Only the serial service loop owns this slot. A builder owns the unbound child
// until join; accepting/authenticating a channel does not transfer input rights.
type serviceInputPreparation struct {
	next    *preparedConsole
	warming *cursorInputPreparation
}

func (p *serviceInputPreparation) reenter(ctx context.Context, lease Lease, result error, prepare func(context.Context) (*preparedConsole, error)) error {
	if !inputReentryAllowed(ctx, lease, result) {
		return nil
	}
	// A long active lease can outlive the one speculative candidate. Harvest
	// only completed work, then replace an expired candidate through start's
	// existing retirement fence. Never restart periodically while input runs.
	if err := p.collectReady(); err != nil {
		return err
	}
	p.start(ctx, prepare)
	return nil
}

func (p *serviceInputPreparation) start(ctx context.Context, prepare func(context.Context) (*preparedConsole, error)) bool {
	if p.warming != nil || p.next != nil && !p.next.retired() {
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
	return true
}

// Called only after an authenticated hint's receipt and channel close. The
// receipt acknowledges demand, not readiness. Keep one bounded builder, and
// retain the existing cooldown/expiry policy without tying work to an ACK wait.
func (p *serviceInputPreparation) hint(ctx context.Context, last, now time.Time, prepare func(context.Context) (*preparedConsole, error)) bool {
	if ctx == nil || ctx.Err() != nil || p.warming != nil || !preparationAllowed(last, now) {
		return false
	}
	if p.next != nil && !p.next.retired() && !preparationRefreshDue(last, p.next.preparedAt, now) {
		return false
	}
	retiring := p.next
	p.next = nil
	return p.start(ctx, func(run context.Context) (*preparedConsole, error) {
		if err := retiring.close(); err != nil {
			return nil, errors.Join(ErrRecovery, err)
		}
		return prepare(run)
	})
}

func (p *serviceInputPreparation) join() error {
	if p.warming == nil {
		return nil
	}
	var err error
	p.next, err = p.warming.join()
	p.warming = nil
	// Ordinary construction failure still permits the existing cold fallback.
	return serviceRecoveryFailure(err, nil)
}

// Harvest only a completed builder. Neither optional demand nor authentication
// may wait for native construction, but completed errors must fence admission.
func (p *serviceInputPreparation) collectReady() error {
	if p.warming != nil {
		select {
		case <-p.warming.done:
			return p.join()
		default:
		}
	}
	return nil
}

func (p *serviceInputPreparation) accept(accept func() (*Channel, error)) (*Channel, error) {
	hadPreparation := p.warming != nil
	if err := p.collectReady(); err != nil {
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
	if p.warming != nil {
		started := time.Now()
		err := p.join()
		channel.preparationWait = time.Since(started)
		channel.preparationWaitMeasured = true
		if err != nil {
			return serviceRecoveryFailure(err, channel.Close())
		}
	}
	return nil
}

func (p *serviceInputPreparation) close() error {
	if p.warming != nil {
		p.warming.cancel()
	}
	err := p.join()
	closeErr := p.next.close()
	p.next = nil
	return serviceRecoveryFailure(err, closeErr)
}
