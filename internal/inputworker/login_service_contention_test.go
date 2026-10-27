package inputworker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
)

func TestLoginServicePolicyReadDoesNotDelayInputAdmission(t *testing.T) {
	t.Run("policy-read", func(t *testing.T) {
		var state atomic.Uint32
		state.Store(uint32(loginagent.UserSession))
		var starts atomic.Int32
		gate := serviceLoginGate(t, &state, &serviceLoginWorker{done: make(chan struct{})}, &starts)
		defer gate.owner.Close()
		reading, done := make(chan struct{}), make(chan error, 1)
		gate.selection = func() (loginagent.Selection, error) {
			close(reading)
			time.Sleep(317 * time.Millisecond)
			return loginagent.Selection{Enabled: true, Profile: [32]byte{1}}, nil
		}
		go func() { done <- gate.reconcile(context.Background()) }()
		<-reading
		started := time.Now()
		if err := gate.admit(context.Background()); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
			t.Errorf("input admission waited behind unrelated policy I/O: %s", elapsed)
		}
		if err := <-done; err != nil || starts.Load() != 0 {
			t.Fatal("logged-in console started a pre-login worker", starts.Load(), err)
		}
	})
}

func TestLoginServiceDelayedSelectionStillRechecksConsole(t *testing.T) {
	t.Run("policy-read", func(t *testing.T) {
		var state atomic.Uint32
		state.Store(uint32(loginagent.BeforeLogin))
		var starts atomic.Int32
		gate := serviceLoginGate(t, &state, &serviceLoginWorker{done: make(chan struct{})}, &starts)
		defer gate.owner.Close()
		reading, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		gate.selection = func() (loginagent.Selection, error) {
			close(reading)
			<-release
			return loginagent.Selection{Enabled: true, Profile: [32]byte{1}}, nil
		}
		go func() { done <- gate.reconcile(context.Background()) }()
		<-reading
		state.Store(uint32(loginagent.UserSession))
		// Finish the metadata read only after the foreground admission has had
		// a chance to retire the old owner. The delayed selection is no proof.
		admitted := make(chan error, 1)
		go func() { admitted <- gate.admit(context.Background()) }()
		select {
		case err := <-admitted:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("policy read owns the admission gate")
		}
		close(release)
		if err := <-done; err != nil || starts.Load() != 0 {
			t.Fatal("stale selection authorized a pre-login start", starts.Load(), err)
		}
	})
}
