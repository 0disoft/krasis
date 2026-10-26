package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputworker"
)

func TestReturnCursorExpiryJoinsCloseBeforeResumingPreparation(t *testing.T) {
	for _, expires := range []time.Duration{100 * time.Millisecond, time.Hour} {
		t.Run(expires.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root, cancel := context.WithCancel(context.Background())
				defer cancel()
				input := &privilegedLeaseInput{client: &testCursorClient{}, identity: application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, leaseExpires: time.Now().Add(expires)}
				input.cursor.root = root
				var opens, closes, prepares int
				closeStarted, releaseClose := make(chan struct{}), make(chan struct{})
				input.cursor.open = func(ctx context.Context, lease inputworker.Lease) (func() error, error) {
					opens++
					return func() error {
						closes++
						close(closeStarted)
						<-releaseClose
						return nil
					}, nil
				}
				input.prepare = func(context.Context) error { prepares++; return nil }
				action := input.PrepareReturnCursor()
				if action == nil {
					t.Fatal("missing cursor snapshot")
				}
				input.close(nil) // Presentation follows the completed input lease.
				action()
				synctest.Wait()
				done := input.cursor.done
				limit := min(expires, time.Minute)
				time.Sleep(limit)
				synctest.Wait()
				select {
				case <-closeStarted:
				default:
					t.Error("expired presentation retained its parent channel and handles")
					cancel() // Let the old implementation finish without a test hang.
					<-closeStarted
				}
				if prepared, err := input.prepareIdle(context.Background()); prepared || err != nil || prepares != 0 || input.cursor.cancel == nil {
					t.Error("preparation bypassed an unjoined native presentation owner", prepared, err)
				}
				close(releaseClose)
				<-done
				synctest.Wait()
				if input.cursor.cancel != nil || input.cursor.done != nil || opens != 1 || closes != 1 {
					t.Error("confirmed expiry did not clear its fence exactly once", opens, closes)
				}
				if prepared, err := input.prepareIdle(context.Background()); !prepared || err != nil || prepares != 1 {
					t.Error("expired presentation still blocked idle preparation", prepared, err)
				}
				action() // The old return receipt cannot create a second presentation.
				synctest.Wait()
				if opens != 1 || input.cursor.cancel != nil {
					t.Error("completed return action reopened stale presentation")
				}
				input.RestoreReturnCursor()
			})
		})
	}
}

func TestReturnCursorExpiryPreservesFailedCleanupFence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root, cancel := context.WithCancel(context.Background())
		defer cancel()
		closeErr := errors.New("native cleanup unconfirmed")
		input := &privilegedLeaseInput{client: &testCursorClient{}, identity: application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, leaseExpires: time.Now().Add(100 * time.Millisecond)}
		input.cursor.root = root
		input.cursor.open = func(context.Context, inputworker.Lease) (func() error, error) {
			return func() error { return closeErr }, nil
		}
		action := input.PrepareReturnCursor()
		input.close(nil)
		action()
		synctest.Wait()
		done := input.cursor.done
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Error("expired failed presentation did not attempt cleanup")
			cancel()
			<-done
		}
		if !errors.Is(input.closeErr, closeErr) || input.cursor.cancel == nil || input.cursor.done != done {
			t.Error("failed cleanup was reported as clean retirement", input.closeErr)
		}
		input.RestoreReturnCursor()
		identity := application.LeaseIdentity{SessionID: "new", LeaseID: "l", Epoch: "e"}
		if err := input.open(context.Background(), identity, time.Now()); !errors.Is(err, closeErr) {
			t.Error("new input lease ignored the terminal cleanup failure", err)
		}
	})
}
