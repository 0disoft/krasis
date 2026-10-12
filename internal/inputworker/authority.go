package inputworker

import (
	"context"
	"sync"
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
	mu                 sync.Mutex
	closed             bool
	checkInput         func() error
	checkInputObserved func(context.Context) error
	checkRecovery      func() error
	closeHandles       func() error
}

func (authority *Authority) check(recovery bool) error {
	return authority.checkObserved(nil, recovery)
}

// The context only selects diagnostic recording for an actual client input.
// Closed/zero proofs and recovery use the same mandatory checks as before.
func (authority *Authority) checkObserved(ctx context.Context, recovery bool) error {
	if authority == nil {
		return ErrAuthority
	}
	var started time.Time
	if ctx != nil && !recovery {
		started = time.Now()
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if !started.IsZero() {
		handoff.RecordAuthority(ctx, handoff.AuthorityLock, time.Since(started))
	}
	if authority.closed || authority.checkInput == nil || authority.checkRecovery == nil || authority.closeHandles == nil {
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
	if err != nil {
		return ErrAuthority
	}
	return nil
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
