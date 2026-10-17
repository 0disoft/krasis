package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

// Model a service child which is already starting when an approved entry
// arrives. Canceling the hint throws away useful construction and cold-launches
// again even though the original preparation has a short remaining deadline.
func TestApprovedEntryReusesPreparationAlreadyInsideItsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		prepareCtx, stopPrepare := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stopPrepare()
		entered, finished := make(chan struct{}), make(chan struct{})
		var warm atomic.Bool
		var attempts atomic.Uint32
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour),
			prepare: func(run context.Context) error {
				attempts.Add(1)
				close(entered)
				select {
				case <-run.Done():
					return run.Err()
				case <-time.After(120 * time.Millisecond):
					warm.Store(true)
					return nil
				}
			},
			connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
				if !warm.Load() {
					return nil, errors.New("approved entry discarded its in-flight warm child")
				}
				return &testPrivilegedClient{}, nil
			},
		}
		defer input.shutdown()
		go func() { _, _ = input.prepareIdle(prepareCtx); close(finished) }()
		<-entered
		start := time.Now()
		err := input.open(ctx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
		<-finished
		if err != nil || !warm.Load() || attempts.Load() != 1 || time.Since(start) != 120*time.Millisecond {
			t.Fatal("pending preparation was canceled/restarted instead of joined", err, attempts.Load(), time.Since(start))
		}
	})
}

func TestPreparationAdmissionCancellationCannotStartLeaseOrBypassCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prepCtx, stopPrep := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer stopPrep()
		entryCtx, stopEntry := context.WithCancel(context.Background())
		defer stopEntry()
		entered, canceled, cleanup, prepared := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour),
			prepare: func(run context.Context) error {
				close(entered)
				<-run.Done()
				close(canceled)
				<-cleanup
				return run.Err()
			},
			connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
				t.Error("canceled admission reached service connect")
				return nil, errPrivilegedInput
			},
		}
		go func() { _, _ = input.prepareIdle(prepCtx); close(prepared) }()
		<-entered
		result := make(chan error, 1)
		go func() {
			result <- input.open(entryCtx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
		}()
		synctest.Wait()
		stopEntry()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Error("entry cancellation was not preserved", err)
		}
		<-canceled
		if input.mu.TryLock() {
			input.mu.Unlock()
			t.Error("native cleanup no longer owns its fence")
		}
		close(cleanup)
		<-prepared
		input.shutdown()
	})
}

func TestRejectedLeaseDoesNotWaitForOrCancelPreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		entered, finished, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), prepare: func(run context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-run.Done():
				return run.Err()
			}
		}}
		go func() { _, _ = input.prepareIdle(ctx); close(finished) }()
		<-entered
		a := privilegedAuthorizer{LeaseAuthorizer: testPrivilegedAuthorizer{accepted: false}, input: input}
		_, accepted, err := a.AuthorizeLease(ctx, transport.AuthenticatedPeerContext{}, "s", "e", time.Now())
		state := input.preparationCancel.Load()
		if accepted || err != nil || state == nil || ctx.Err() != nil {
			t.Error("rejected entry affected pending preparation", err)
		}
		close(release)
		<-finished
		input.shutdown()
	})
}

func TestExpiredPreparationFallsBackWithoutExtendingOriginalBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		prepareCtx, stopPrepare := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stopPrepare()
		entered, prepared := make(chan struct{}), make(chan struct{})
		input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), prepare: func(run context.Context) error {
			close(entered)
			<-run.Done()
			return run.Err()
		}, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
			return &testPrivilegedClient{}, nil
		}}
		defer input.shutdown()
		go func() { _, _ = input.prepareIdle(prepareCtx); close(prepared) }()
		<-entered
		start := time.Now()
		err := input.open(ctx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
		<-prepared
		if err != nil || time.Since(start) != 300*time.Millisecond || prepareCtx.Err() != context.DeadlineExceeded {
			t.Fatal("original preparation deadline changed or fallback failed", err, time.Since(start))
		}
	})
}
