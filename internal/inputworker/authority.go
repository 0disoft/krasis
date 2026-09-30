package inputworker

import "sync"

type authorityKind uint8

const (
	authorityAgent authorityKind = iota + 1
	authorityService
)

// Authority can only be created by a platform authenticator in this package.
// A zero value never permits input. Its kernel process handle pins one process
// lifetime; client-provided PIDs and claims cannot create this proof.
type Authority struct {
	kind          authorityKind
	mu            sync.Mutex
	closed        bool
	checkInput    func() error
	checkRecovery func() error
	closeHandles  func() error
}

func (authority *Authority) check(recovery bool) error {
	if authority == nil {
		return ErrAuthority
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.closed || authority.checkInput == nil || authority.checkRecovery == nil || authority.closeHandles == nil {
		return ErrAuthority
	}
	check := authority.checkInput
	if recovery {
		check = authority.checkRecovery
	}
	if check() != nil {
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
