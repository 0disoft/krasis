package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestReturnCursorRetirementHonorsEntryBudgetAndRetainsFence(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var connections int
				input := &privilegedLeaseInput{connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
					connections++
					return &testPrivilegedClient{}, nil
				}}
				owner, stopOwner := context.WithCancel(context.Background())
				defer stopOwner()
				done := make(chan struct{})
				input.cursor.cancel, input.cursor.done = stopOwner, done
				go func() {
					<-owner.Done()
					// Native close still owns the presentation resources after cancel.
					time.Sleep(500 * time.Millisecond)
					close(done)
				}()
				identity := application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}
				ctx, cancel := context.WithCancel(context.Background())
				want := error(context.Canceled)
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
					want = context.DeadlineExceeded
				} else {
					timer := time.AfterFunc(50*time.Millisecond, cancel)
					defer timer.Stop()
				}
				defer cancel()
				started := time.Now()
				err := input.open(ctx, identity, time.Now())
				if !errors.Is(err, want) || !errors.Is(err, errPrivilegedInput) || time.Since(started) != 50*time.Millisecond {
					t.Errorf("entry waited beyond its budget: elapsed=%s err=%v", time.Since(started), err)
				}
				if input.cursor.cancel == nil || input.cursor.done != done || connections != 0 || input.client != nil || input.entryWaiting.Load() != 0 {
					t.Error("unconfirmed retirement lost its fence or admitted a new input lease")
				}
				// A second entry must still join the SAME native owner, not clear
				// the fence or spawn a replacement after the first caller left.
				next, stopNext := context.WithTimeout(context.Background(), 20*time.Millisecond)
				started = time.Now()
				err = input.open(next, identity, time.Now())
				stopNext()
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 20*time.Millisecond || connections != 0 {
					t.Error("subsequent entry bypassed the retained native owner", err, connections)
				}
				<-done
				input.RestoreReturnCursor()
				if input.cursor.cancel != nil || input.cursor.done != nil {
					t.Error("confirmed retirement did not release its fence")
				}
				if err := input.open(context.Background(), identity, time.Now()); err != nil || connections != 1 {
					t.Fatal("entry failed to resume after confirmed native retirement", err, connections)
				}
				if err := input.shutdown(); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
