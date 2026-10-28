package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
)

func TestIdleLoginAdmissionUsesOneFreshConsoleObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var state atomic.Uint32
		state.Store(uint32(loginagent.UserSession))
		var starts atomic.Int32
		gate := serviceLoginGate(t, &state, &serviceLoginWorker{done: make(chan struct{})}, &starts)
		defer gate.owner.Close()
		reads := 0
		gate.console = func() (loginagent.Console, error) {
			reads++
			time.Sleep(40 * time.Millisecond) // A delayed read-only WTS RPC.
			return loginagent.Console{Session: 1, State: loginagent.UserSession}, nil
		}
		for range 2 {
			started := time.Now()
			if err := gate.admit(context.Background()); err != nil {
				t.Fatal(err)
			}
			if elapsed := time.Since(started); elapsed != 40*time.Millisecond {
				t.Fatal("idle admission repeated a native query", elapsed)
			}
		}
		if reads != 2 || starts.Load() != 0 {
			t.Fatal("admission cached an observation or started an owner", reads, starts.Load())
		}
	})
}

func TestLoginAdmissionRechecksConsoleAcrossActualRetirement(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.BeforeLogin))
	var starts atomic.Int32
	worker := &serviceLoginWorker{done: make(chan struct{}), stopEntered: make(chan struct{}), release: make(chan struct{})}
	gate := serviceLoginGate(t, &state, worker, &starts)
	defer gate.owner.Close()
	if err := gate.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	state.Store(uint32(loginagent.UserSession))
	var session atomic.Uint32
	session.Store(1)
	var reads atomic.Int32
	gate.console = func() (loginagent.Console, error) {
		reads.Add(1)
		return loginagent.Console{Session: session.Load(), State: loginagent.UserSession}, nil
	}
	result := make(chan error, 1)
	go func() { result <- gate.admit(context.Background()) }()
	<-worker.stopEntered
	session.Store(2) // The console changes while the old worker is joining.
	close(worker.release)
	if err := <-result; !errors.Is(err, ErrAuthority) || reads.Load() != 2 {
		t.Fatal("retirement reused a stale console observation", err, reads.Load())
	}
}

func TestIdleLoginAdmissionDoesNotRetainPreviousConsoleSuccess(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.UserSession))
	var starts atomic.Int32
	gate := serviceLoginGate(t, &state, &serviceLoginWorker{done: make(chan struct{})}, &starts)
	defer gate.owner.Close()
	if err := gate.admit(context.Background()); err != nil {
		t.Fatal(err)
	}
	state.Store(uint32(loginagent.BeforeLogin))
	if err := gate.admit(context.Background()); !errors.Is(err, ErrAuthority) {
		t.Fatal("a previous console success authorized a new admission", err)
	}
}
