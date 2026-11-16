package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestConsoleObservationCannotBlockAdmissionOnRuntimePersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		var saved []logindiag.Runtime
		r := newLoginRuntimeRecorder(func(v logindiag.Runtime) error {
			if len(saved) == 0 {
				close(entered)
				time.Sleep(220 * time.Millisecond)
			}
			saved = append(saved, v)
			return nil
		})
		done := make(chan struct{})
		go func() { r.record("service", "READY", nil); close(done) }()
		<-entered
		var state atomic.Uint32
		state.Store(uint32(loginagent.UserSession))
		var starts atomic.Int32
		gate := serviceLoginGate(t, &state, &serviceLoginWorker{done: make(chan struct{})}, &starts)
		defer gate.owner.Close()
		gate.console = func() (loginagent.Console, error) {
			r.observeConsole("USER_SESSION", nil)
			return loginagent.Console{Session: 1, State: loginagent.UserSession}, nil
		}
		started := time.Now()
		want := time.Duration(0)
		if err := gate.admit(context.Background()); err != nil || time.Since(started) != want {
			t.Fatal("diagnostic storage delayed fresh console admission", err, time.Since(started))
		}
		gate.console = func() (loginagent.Console, error) {
			r.observeConsole("UNKNOWN", loginagent.ErrObservation)
			return loginagent.Console{}, loginagent.ErrObservation
		}
		if err := gate.admit(context.Background()); !errors.Is(err, ErrAuthority) {
			t.Fatal("pending diagnostics authorized a failed console query", err)
		}
		<-done
		r.record("service", "STOPPED", nil)
		last := saved[len(saved)-1]
		if last.Console != "UNKNOWN" || last.LastFailure == nil || last.LastFailure.Stage != "console" || starts.Load() != 0 {
			t.Fatal("final recording lost console failure", last)
		}
	})
}

func TestConsoleSnapshotKeepsFailureAndRetriesFailedPersistence(t *testing.T) {
	var saved []logindiag.Runtime
	fail := true
	r := newLoginRuntimeRecorder(func(v logindiag.Runtime) error {
		if fail {
			return errors.New("disk unavailable")
		}
		saved = append(saved, v)
		return nil
	})
	r.observeConsole("UNKNOWN", syscall.Errno(5))
	r.observeConsole("USER_SESSION", nil)
	r.record("service", "READY", nil)
	if r.observedConsole != nil || r.value.Time != "" {
		t.Fatal("failed write marked snapshot persisted")
	}
	fail = false
	r.record("service", "READY", nil)
	last := saved[len(saved)-1]
	if !last.Valid() || last.Console != "USER_SESSION" || last.LastFailure == nil || last.LastFailure.Win32Error != 5 {
		t.Fatal("success erased unpublished console failure", last)
	}
	r.record("start", "FAILED", syscall.Errno(6))
	r.observeConsole("BEFORE_LOGIN", nil)
	r.record("service", "STOPPED", nil)
	if saved[len(saved)-1].LastFailure.Win32Error != 6 {
		t.Fatal("old console failure replaced newer worker failure")
	}
}
