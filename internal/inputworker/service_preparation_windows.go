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

func (p *serviceInputPreparation) reenter(ctx context.Context, lease Lease, result error, prepare func(context.Context) (*preparedConsole, error)) {
	if p.warming != nil || p.next != nil {
		return
	}
	p.warming = prepareInputReentry(ctx, lease, result, prepare)
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

func (p *serviceInputPreparation) accept(accept func() (*Channel, error)) (*Channel, error) {
	// A failure already known at loop entry must not wait for another client.
	if p.warming != nil {
		select {
		case <-p.warming.done:
			if err := p.join(); err != nil {
				return nil, err
			}
		default:
		}
	}
	channel, err := accept()
	if err != nil {
		return channel, err
	}
	// Authentication can overlap construction, but admission, request handling
	// and all subsequent launches remain behind this recovery fence.
	if p.warming != nil {
		started := time.Now()
		err = p.join()
		channel.preparationWait = time.Since(started)
		channel.preparationWaitMeasured = true
		if err != nil {
			return nil, serviceRecoveryFailure(err, channel.Close())
		}
	}
	return channel, nil
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
