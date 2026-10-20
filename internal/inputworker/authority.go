package inputworker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type authorityKind uint8

const (
	authorityAgent authorityKind = iota + 1
	authorityService
)

// Authority can only be created by a platform authenticator in this package.
// A zero value never permits input. Its kernel process handle pins one process
// lifetime; client-provided PIDs and claims cannot create this proof.
type Authority struct {
	kind               authorityKind
	agentStarted       uint64 // Kernel process creation time, immutable auth evidence.
	mu                 sync.RWMutex
	closed             bool
	fenced             atomic.Bool // Deny immediately while a timed-out query still owns handles.
	checkInput         func() error
	checkInputObserved func(context.Context) error
	checkRecovery      func() error
	checkPresent       func() error // Heartbeat only; never authorizes input or recovery.
	closeHandles       func() error
}

// A keepalive does not grant capture, inject an action, or renew peer trust.
// Native authenticators can check their pinned kernel owner without repeating
// SCM/WTS RPC. All state-changing commands and input still use check(false).
// Authorities without a separate presence proof retain the full check.
func (authority *Authority) checkHeartbeat() error {
	if authority.heartbeatResult() != nil {
		return ErrAuthority
	}
	return nil
}

func (authority *Authority) checkSourceHeartbeat() error {
	return sourceAuthorityResult(authority.heartbeatResult())
}

func (authority *Authority) heartbeatResult() error {
	if authority == nil || authority.fenced.Load() {
		return ErrAuthority
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.fenced.Load() || authority.closed || authority.checkInput == nil || authority.checkRecovery == nil || authority.closeHandles == nil {
		return ErrAuthority
	}
	check := authority.checkPresent
	if check == nil {
		check = authority.checkInput
	}
	return check()
}

func (authority *Authority) check(recovery bool) error {
	return authority.checkObserved(nil, recovery)
}

// The context only selects diagnostic recording for an actual client input.
// Closed/zero proofs and recovery use the same mandatory checks as before.
func (authority *Authority) checkObserved(ctx context.Context, recovery bool) error {
	if authority.checkResult(ctx, recovery) != nil {
		return ErrAuthority
	}
	return nil
}

// Source reports retain only a closed operation code and numeric native error.
// Ordinary callers keep the existing sentinel-only error contract.
func (authority *Authority) checkSource() error {
	// Called by low-level physical input callbacks. A pending Close must deny
	// new capture immediately rather than queue it behind existing SCM/WTS work.
	// Acquired readers still pin all handles until their fresh check returns.
	if authority == nil || authority.fenced.Load() || !authority.mu.TryRLock() {
		return ErrAuthority
	}
	defer authority.mu.RUnlock()
	return sourceAuthorityResult(authority.checkLocked(nil, false))
}

func (authority *Authority) checkResult(ctx context.Context, recovery bool) error {
	if authority == nil || authority.fenced.Load() {
		return ErrAuthority
	}
	var started time.Time
	if ctx != nil && !recovery {
		started = time.Now()
	}
	// Checks only query immutable pinned handles; independent readers must not
	// queue behind a slow SCM/WTS query. Close still joins every reader before
	// releasing those handles.
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if !started.IsZero() {
		handoff.RecordAuthority(ctx, handoff.AuthorityLock, time.Since(started))
	}
	return authority.checkLocked(ctx, recovery)
}

// Caller holds mu for the entire native check, including any slow RPC.
func (authority *Authority) checkLocked(ctx context.Context, recovery bool) error {
	if authority.fenced.Load() || authority.closed || authority.checkInput == nil || authority.checkRecovery == nil || authority.closeHandles == nil {
		return ErrAuthority
	}
	check := authority.checkInput
	if recovery {
		check = authority.checkRecovery
	}
	var err error
	if ctx != nil && !recovery && authority.checkInputObserved != nil {
		err = authority.checkInputObserved(ctx)
	} else {
		err = check()
	}
	return err
}

func (authority *Authority) Close() error {
	if authority == nil {
		return nil
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.closed {
		return nil
	}
	authority.closed = true
	if authority.closeHandles == nil {
		return nil
	}
	if authority.closeHandles() != nil {
		return ErrAuthority
	}
	return nil
}
