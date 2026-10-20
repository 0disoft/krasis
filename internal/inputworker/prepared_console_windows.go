//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

// Preparation owns only the job and the SYSTEM-only control pipe. No lease,
// agent input pipe or image is transferred until the existing bootstrap.
type preparedConsole struct {
	worker     *ConsoleWorker
	control    *pipeConn
	pid        uint32
	session    uint32
	profile    handoff.StartupProfile
	preparedAt time.Time
	warmState  atomic.Uint32 // 0: unbound, 1: claimed once, 2: retired, 3: expired
	warmTimer  *time.Timer
}

// A passive presentation launches its own bounded owner. Keep the unbound input
// child for the next lease; its existing expiry/session checks still apply.
func takeInputPreparation(next **preparedConsole, lease Lease) *preparedConsole {
	if lease.Presentation {
		return nil
	}
	candidate := *next
	*next = nil
	return candidate
}

// Only the installed service can prepare an unbound child. It receives no
// agent pipe, lease, image pin or input. The ordinary launch API stays agent-only.
func prepareServiceConsole(ctx context.Context, installation *Installation, digest [sha256.Size]byte) (*preparedConsole, error) {
	if ctx == nil || ctx.Err() != nil || installation == nil {
		return nil, ErrAuthority
	}
	_, service, err := installation.digests()
	if err != nil || service != digest || !fixedInstalledInputProcess(service, false, installation) {
		return nil, ErrAuthority
	}
	p, err := prepareConsole(ctx, func(prepare func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
		return launchManagedConsoleChecked(ctx, digest, prepare, installation, func() bool {
			return fixedInstalledInputProcess(digest, false, installation)
		})
	})
	if err != nil {
		return nil, err
	}
	// Expiry can only destroy an unbound child. Claim and expiry race through
	// one atomic state; the callback can never close an active input lease.
	p.warmTimer = time.AfterFunc(preparedConsoleWindow, func() {
		// Both Close methods retain cleanup errors. The owning service loop
		// must join close() before another child or lease can be created.
		_ = p.expireUnbound()
	})
	return p, nil
}

func preparedConsoleFresh(age time.Duration, session, current uint32, alive bool) bool {
	return preparedConsoleHealth(age, session, current, alive) == handoff.PreparationUsed
}

func (p *preparedConsole) claim(ctx context.Context) bool {
	return p.claimDecision(ctx) == handoff.PreparationUsed
}

func (p *preparedConsole) claimDecision(ctx context.Context) handoff.PreparationDecision {
	if p == nil {
		return handoff.PreparationNoCandidate
	}
	if ctx == nil || ctx.Err() != nil {
		return handoff.PreparationCancelled
	}
	if p.warmTimer == nil {
		return handoff.PreparationUnusable
	}
	if !p.warmState.CompareAndSwap(0, 1) {
		if p.warmState.Load() == 3 {
			return handoff.PreparationExpired
		}
		return handoff.PreparationClaimLost
	}
	p.warmTimer.Stop()
	age := time.Since(p.preparedAt)
	if p.worker == nil || p.control == nil || p.pid == 0 {
		return handoff.PreparationUnusable
	}
	p.worker.mu.Lock()
	unusable := p.worker.closed || p.worker.retiring || p.worker.process == 0
	p.worker.mu.Unlock()
	if unusable {
		return handoff.PreparationUnusable
	}
	decision := preparedConsoleHealth(age, p.session, windows.WTSGetActiveConsoleSessionId(), !p.worker.exited())
	if decision != handoff.PreparationUsed {
		return decision
	}
	if ctx.Err() != nil {
		return handoff.PreparationCancelled
	}
	p.profile.Prepared, p.profile.PreparedAgeMicros = true, uint32(age.Microseconds())
	return handoff.PreparationUsed
}

func (p *preparedConsole) retireUnbound() error {
	return p.retireUnboundState(2)
}

func (p *preparedConsole) expireUnbound() error {
	return p.retireUnboundState(3)
}

func (p *preparedConsole) retired() bool { return p != nil && p.warmState.Load() >= 2 }

func (p *preparedConsole) retireUnboundState(state uint32) error {
	if p == nil || !p.warmState.CompareAndSwap(0, state) {
		return nil
	}
	return p.closeOwned()
}

type consoleLauncher func(func(windows.Handle, uint32) error) (*ConsoleWorker, error)

func prepareAgentConsole(ctx context.Context, agent *Authority, digest [sha256.Size]byte, installation *Installation) (*preparedConsole, error) {
	return prepareConsole(ctx, func(prepare func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
		return launchManagedConsole(ctx, agent, digest, prepare, installation)
	})
}

func prepareConsole(ctx context.Context, launch consoleLauncher) (prepared *preparedConsole, result error) {
	if ctx == nil || ctx.Err() != nil || launch == nil {
		return nil, ErrAuthority
	}
	p := &preparedConsole{}
	accepted := false
	defer func() {
		if !accepted && p.close() != nil {
			result = errors.Join(result, ErrRecovery)
		}
	}()
	started := time.Now()
	worker, err := launch(func(_ windows.Handle, pid uint32) error {
		p.pid, p.session = pid, windows.WTSGetActiveConsoleSessionId()
		var err error
		p.control, err = makePipeACL(workerPipeName(pid), "D:P(A;;GA;;;SY)")
		return err
	})
	p.profile.Record(handoff.ServiceLaunch, time.Since(started))
	if err != nil {
		return nil, err
	}
	p.worker = worker
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	started = time.Now()
	err = p.control.connect(startup)
	p.profile.Record(handoff.ServiceChildConnect, time.Since(started))
	if err != nil || startup.Err() != nil {
		return nil, ErrAuthority
	}
	p.preparedAt = time.Now()
	accepted = true
	return p, nil
}

func (prepared *preparedConsole) close() error {
	if prepared == nil {
		return nil
	}
	prepared.warmState.Store(2)
	if prepared.warmTimer != nil {
		prepared.warmTimer.Stop()
	}
	return prepared.closeOwned()
}

func (prepared *preparedConsole) closeOwned() error {
	var err error
	if prepared.control != nil {
		err = prepared.control.Close()
	}
	if prepared.worker != nil {
		err = errors.Join(err, prepared.worker.Close())
	}
	return err
}
