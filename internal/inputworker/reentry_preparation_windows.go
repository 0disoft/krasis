//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"
)

type reentryPreparation struct{ timer *time.Timer }

// One clean lease retirement permits one delayed construction opportunity.
// The original spare keeps its fixed bounded expiry. Rebuild once after that
// reservation expires rather than requiring another outward movement hint.
// This never renews a child, repeats periodically, or grants input authority.
func (p *serviceInputPreparation) armReentryLocked(ctx context.Context, expires time.Time, prepare func(context.Context) (*preparedConsole, error)) {
	p.cancelReentryLocked()
	token := &reentryPreparation{}
	p.reentry = token
	var fire func()
	fire = func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.reentry != token {
			return
		}
		if ctx.Err() != nil || !expires.After(time.Now()) || p.collectReadyLocked() != nil || p.warming != nil {
			p.reentry = nil
			return
		}
		if p.next != nil && !p.next.retired() {
			// Construction initiated at return may have completed later than
			// this timer's anchor. Wait for that exact spare's original expiry;
			// never retire it early, extend it, or schedule after a replacement.
			if remaining := time.Until(p.next.preparedAt.Add(preparedConsoleWindow)); remaining > 0 {
				token.timer = time.AfterFunc(remaining, fire)
				return
			}
		}
		p.reentry = nil // Consume the single opportunity before launching.
		if !preparationAllowed(p.last, time.Now()) {
			return
		}
		retiring := p.next
		p.next = nil
		p.startLocked(ctx, func(run context.Context) (*preparedConsole, error) {
			if err := retiring.close(); err != nil {
				return nil, errors.Join(ErrRecovery, err)
			}
			return prepare(run)
		})
	}
	token.timer = time.AfterFunc(preparedConsoleWindow, fire)
}

func (p *serviceInputPreparation) cancelReentryLocked() {
	if token := p.reentry; token != nil {
		p.reentry = nil
		token.timer.Stop()
	}
}
