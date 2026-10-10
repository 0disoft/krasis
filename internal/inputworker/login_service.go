package inputworker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
)

// The same gate serializes console polling and admission of an authenticated
// user agent. No ordinary lease may run until pre-login retirement is confirmed.
type loginServiceGate struct {
	mu        sync.Mutex
	owner     *loginagent.Owner
	console   func() (loginagent.Console, error)
	selection func() (loginagent.Selection, error)
}

func (g *loginServiceGate) reconcile(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	selection, err := g.selection()
	if err != nil {
		selection = loginagent.Selection{}
	}
	return errors.Join(err, g.owner.Reconcile(ctx, selection))
}

func (g *loginServiceGate) admit(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	before, err := g.console()
	if err != nil || before.State != loginagent.UserSession || before.Session == 0 || before.Session == ^uint32(0) {
		return ErrAuthority
	}
	if err := g.owner.Reconcile(ctx, loginagent.Selection{}); err != nil {
		return errors.Join(ErrRecovery, err)
	}
	after, err := g.console()
	if err != nil || after != before {
		return ErrAuthority
	}
	return ctx.Err()
}

func runLoginService(ctx context.Context, gate *loginServiceGate, regular func(context.Context, func(context.Context) error) error) (result error) {
	if ctx == nil || gate == nil || gate.owner == nil || gate.console == nil || gate.selection == nil || regular == nil {
		return ErrAuthority
	}
	defer func() { result = errors.Join(result, gate.owner.Close()) }()
	// Ordinary observation/start failures disable only the pre-login owner.
	// Unconfirmed cleanup fences the entire service, including normal leases.
	if err := gate.reconcile(ctx); errors.Is(err, loginagent.ErrRecovery) || errors.Is(err, ErrRecovery) {
		return err
	}
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	monitor := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-lifetime.Done():
				monitor <- nil
				return
			case <-ticker.C:
				if err := gate.reconcile(lifetime); errors.Is(err, loginagent.ErrRecovery) || errors.Is(err, ErrRecovery) {
					cancel()
					monitor <- err
					return
				}
			}
		}
	}()
	result = regular(lifetime, gate.admit)
	cancel()
	return errors.Join(result, <-monitor)
}
