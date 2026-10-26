package residentagent

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
)

func TestApprovedEntryCanReuseLongerIdlePreparationWithinItsOwnWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), agent.InputPreparationTimeout)
		defer cancel()
		entered, finished := make(chan struct{}), make(chan struct{})
		var warm atomic.Bool
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour),
			prepare: func(run context.Context) error {
				close(entered)
				select {
				case <-time.After(500 * time.Millisecond):
					warm.Store(true)
					return nil
				case <-run.Done():
					return run.Err()
				}
			},
			connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
				if !warm.Load() {
					t.Fatal("entry discarded its nearly ready child")
				}
				return &testPrivilegedClient{}, nil
			},
		}
		defer input.shutdown()
		go func() { _, _ = input.prepareIdle(ctx); close(finished) }()
		<-entered
		time.Sleep(250 * time.Millisecond)
		start := time.Now()
		if err := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now()); err != nil {
			t.Fatal(err)
		}
		<-finished
		if !warm.Load() || time.Since(start) != 250*time.Millisecond {
			t.Fatal("idle lifetime became an entry wait or construction restarted", time.Since(start))
		}
	})
}

func TestEntryCapsLongPreparationAndStillJoinsNativeCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), agent.InputPreparationTimeout)
		defer cancel()
		entered, finished := make(chan struct{}), make(chan struct{})
		var cleaned atomic.Bool
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour),
			prepare: func(run context.Context) error {
				close(entered)
				<-run.Done()
				time.Sleep(40 * time.Millisecond) // Model owned native retirement.
				cleaned.Store(true)
				return run.Err()
			},
			connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
				if !cleaned.Load() {
					t.Fatal("cold fallback bypassed cleanup")
				}
				return &testPrivilegedClient{}, nil
			},
		}
		defer input.shutdown()
		go func() { _, _ = input.prepareIdle(ctx); close(finished) }()
		<-entered
		start := time.Now()
		if err := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now()); err != nil {
			t.Fatal(err)
		}
		<-finished
		if time.Since(start) != agent.InputPreparationEntryWait+40*time.Millisecond {
			t.Fatal("entry waited through optional lifetime or skipped cleanup", time.Since(start))
		}
	})
}
