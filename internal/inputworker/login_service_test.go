package inputworker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
)

type serviceLoginWorker struct {
	ctx                  context.Context
	done                 chan struct{}
	stopEntered, release chan struct{}
	stopErr              error
	once                 sync.Once
}

func (w *serviceLoginWorker) Done() <-chan struct{} { return w.done }
func (w *serviceLoginWorker) Stop(ctx context.Context) error {
	if ctx.Err() != nil || w.ctx.Err() == nil {
		return errors.New("retirement context/lifetime invalid")
	}
	if w.stopEntered != nil {
		w.once.Do(func() { close(w.stopEntered) })
	}
	if w.release != nil {
		select {
		case <-w.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if w.stopErr != nil {
		return w.stopErr
	}
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	return nil
}

func serviceLoginGate(t *testing.T, state *atomic.Uint32, worker *serviceLoginWorker, starts *atomic.Int32) *loginServiceGate {
	t.Helper()
	read := func() (loginagent.Console, error) {
		return loginagent.Console{Session: 1, State: loginagent.ConsoleState(state.Load())}, nil
	}
	owner, err := loginagent.New(loginagent.Dependencies{Read: read, Start: func(ctx context.Context, _ uint32, _ [32]byte) (loginagent.Worker, error) {
		starts.Add(1)
		worker.ctx = ctx
		return worker, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return &loginServiceGate{owner: owner, console: read, selection: func() (loginagent.Selection, error) {
		return loginagent.Selection{Enabled: true, Profile: [32]byte{1}}, nil
	}}
}

func TestLoginServiceHandoffWaitsForOwnedWorkerCleanup(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.BeforeLogin))
	var starts atomic.Int32
	worker := &serviceLoginWorker{done: make(chan struct{}), stopEntered: make(chan struct{}), release: make(chan struct{})}
	gate := serviceLoginGate(t, &state, worker, &starts)
	defer gate.owner.Close()
	if err := gate.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := gate.admit(context.Background()); !errors.Is(err, ErrAuthority) {
		t.Fatal("normal input admitted before login", err)
	}
	state.Store(uint32(loginagent.UserSession))
	admitted := make(chan error, 1)
	go func() { admitted <- gate.admit(context.Background()) }()
	<-worker.stopEntered
	select {
	case err := <-admitted:
		t.Fatalf("handoff before cleanup: %v", err)
	default:
	}
	close(worker.release)
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}
	if err := gate.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 1 {
		t.Fatal("locked/logged-in console started another login worker")
	}
}

func TestLoginServiceCleanupFailureStopsRegularLoopAndFencesAdmission(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.BeforeLogin))
	var starts atomic.Int32
	worker := &serviceLoginWorker{done: make(chan struct{}), stopErr: ErrRecovery}
	gate := serviceLoginGate(t, &state, worker, &starts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := runLoginService(ctx, gate, func(lifetime context.Context, _ func(context.Context) error) error {
		state.Store(uint32(loginagent.UserSession))
		<-lifetime.Done()
		return nil
	})
	if !errors.Is(err, loginagent.ErrRecovery) {
		t.Fatal("failed cleanup hidden", err)
	}
	if err := gate.admit(context.Background()); !errors.Is(err, ErrRecovery) {
		t.Fatal("normal lease escaped failed login cleanup", err)
	}
}

func TestLoginServiceAbsentGrantNeverStartsAndStillAdmitsUser(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.UserSession))
	var starts atomic.Int32
	worker := &serviceLoginWorker{done: make(chan struct{})}
	gate := serviceLoginGate(t, &state, worker, &starts)
	gate.selection = func() (loginagent.Selection, error) { return loginagent.Selection{}, nil }
	if err := runLoginService(context.Background(), gate, func(ctx context.Context, admit func(context.Context) error) error { return admit(ctx) }); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 0 {
		t.Fatal("missing grant authorized a start")
	}
}

func TestLoginServiceBadGrantRetiresWithoutFallbackStart(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.BeforeLogin))
	var starts atomic.Int32
	worker := &serviceLoginWorker{done: make(chan struct{})}
	gate := serviceLoginGate(t, &state, worker, &starts)
	defer gate.owner.Close()
	if err := gate.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	gate.selection = func() (loginagent.Selection, error) { return loginagent.Selection{}, loginagent.ErrGrant }
	if err := gate.reconcile(context.Background()); !errors.Is(err, loginagent.ErrGrant) {
		t.Fatal(err)
	}
	select {
	case <-worker.done:
	default:
		t.Fatal("invalid grant left old worker alive")
	}
	if starts.Load() != 1 {
		t.Fatal("invalid grant started fallback worker")
	}
}
