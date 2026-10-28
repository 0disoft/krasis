// Package loginagent owns the lifetime gate for a service-managed
// pre-login connection. It does not grant trust, inject input or launch a binary.
package loginagent

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid login agent dependencies")
	ErrObservation = errors.New("console login state unavailable")
	ErrStart       = errors.New("login agent start failed")
	ErrRecovery    = errors.New("login agent retirement unconfirmed")
)

type ConsoleState uint8

const (
	Unknown ConsoleState = iota
	Unavailable
	BeforeLogin
	UserSession
)

// Console excludes names, SIDs, PINs and screen contents. A locked user session
// remains UserSession; it is never treated as a fresh pre-login console.
type Console struct {
	Session uint32
	State   ConsoleState
}

func (c Console) eligible() bool {
	return c.State == BeforeLogin && c.Session != 0 && c.Session != ^uint32(0)
}

// Selection is a scheduling input, not authority. Profile must identify a
// separately verified machine-owned grant. A user-writable settings digest
// cannot authorize Start. Changing Profile requires retirement of the old owner.
type Selection struct {
	Enabled bool
	Profile [32]byte
}

type Worker interface {
	// Done closes after the terminal cleanup attempt has joined all owned work.
	// Stop must confirm its outcome before another owner is allowed. Process
	// exit or this signal alone is never proof of successful input retirement.
	Done() <-chan struct{}
	Stop(context.Context) error
}

type Dependencies struct {
	Read func() (Console, error)
	// Start must validate the protected grant, fixed image, actual console and
	// child token independently. It must return promptly on cancellation and
	// clean up any partial start before returning an error.
	Start func(context.Context, uint32, [32]byte) (Worker, error)
}

type target struct {
	console Console
	profile [32]byte
}

// Owner serializes starts and retirements. A failed start or unexpected exit is
// not retried by polling; a newly provisioned grant or service restart is needed.
// A retirement failure permanently fences this owner, including new grants.
type Owner struct {
	mu      sync.Mutex
	deps    Dependencies
	worker  Worker
	done    <-chan struct{}
	cancel  context.CancelFunc
	current target
	blocked target
	fenced  bool
	closed  bool
}

func New(deps Dependencies) (*Owner, error) {
	if deps.Read == nil || deps.Start == nil {
		return nil, ErrInvalid
	}
	return &Owner{deps: deps}, nil
}

func (o *Owner) Reconcile(ctx context.Context, selection Selection) error {
	if o == nil || ctx == nil {
		return ErrInvalid
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.fenced {
		return ErrRecovery
	}
	if o.closed {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, o.retire())
	}
	if !selection.Enabled || selection.Profile == ([32]byte{}) {
		return o.retire()
	}
	if o.done != nil {
		select {
		case <-o.done:
			o.blocked = o.current
			// Even a terminal worker must confirm its cleanup through Stop.
			if err := o.retire(); err != nil {
				return err
			}
			return ErrStart
		default:
		}
	}
	console, err := o.deps.Read()
	if err != nil || console.State == Unknown || console.State > UserSession {
		return errors.Join(ErrObservation, o.retire())
	}
	if !console.eligible() {
		return o.retire()
	}
	next := target{console, selection.Profile}
	if o.worker != nil && o.current == next {
		return nil
	}
	if err := o.retire(); err != nil {
		return err
	}
	if o.blocked == next {
		return ErrStart
	}
	// Retirement can take time. Never start from a snapshot taken before it.
	confirmed, err := o.deps.Read()
	if err != nil || confirmed != console {
		return ErrObservation
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	// Request cancellation may interrupt startup, but cannot later revoke the
	// lifetime of a successfully adopted worker.
	stopCancel := context.AfterFunc(ctx, cancel)
	worker, startErr := o.deps.Start(lifetime, console.Session, selection.Profile)
	stopped := stopCancel()
	if worker != nil {
		o.worker, o.done, o.cancel, o.current = worker, worker.Done(), cancel, next
	} else {
		cancel()
	}
	if startErr != nil || worker == nil || o.done == nil || !stopped || ctx.Err() != nil {
		o.blocked = next
		return errors.Join(ErrStart, startErr, o.retire())
	}
	// Catch a logon/session switch during launch before retaining ownership.
	confirmed, err = o.deps.Read()
	if err != nil || confirmed != console {
		return errors.Join(ErrObservation, o.retire())
	}
	return nil
}

// Retire joins an existing pre-login owner without starting a replacement.
// The boolean reports whether retirement work was required under the owner
// lock. It is not a console observation or permission to inject input.
func (o *Owner) Retire(ctx context.Context) (bool, error) {
	if o == nil || ctx == nil {
		return false, ErrInvalid
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.fenced {
		return false, ErrRecovery
	}
	if o.closed {
		return false, ErrInvalid
	}
	hadWorker := o.worker != nil
	return hadWorker, errors.Join(ctx.Err(), o.retire())
}

func (o *Owner) retire() error {
	if o.worker == nil {
		return nil
	}
	o.cancel()
	// Caller cancellation must not suppress key-up/network retirement.
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.worker.Stop(cleanup); err != nil {
		o.fenced = true
		return errors.Join(ErrRecovery, err)
	}
	if o.done == nil {
		o.fenced = true
		return ErrRecovery
	}
	select {
	case <-o.done:
	case <-cleanup.Done():
		o.fenced = true
		return ErrRecovery
	}
	o.worker, o.done, o.cancel, o.current = nil, nil, nil, target{}
	return nil
}

func (o *Owner) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = true
	if o.fenced {
		return ErrRecovery
	}
	return o.retire()
}
