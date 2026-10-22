package inputworker

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourcePresenceStopDuringQueryStillRequiresNativeRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entered, release := make(chan struct{}), make(chan struct{})
		owner.authority.checkPresent = func() error {
			close(entered)
			<-release
			return nil
		}
		go func() { <-entered; cancel() }()
		err := watchSourcePresence(ctx, func() error { return sourceQueryOwner("agent", owner.authorizeSourcePresence(ctx)) })
		if err != nil {
			t.Error("local stop became authority failure", err)
		}
		// Cancellation cannot certify a still-running native query's cleanup.
		if !owner.authority.fenced.Load() || owner.Close() != ErrAuthority {
			t.Error("cancelled watcher erased native ownership")
		}
		close(release)
		synctest.Wait()
		if owner.Close() != nil {
			t.Fatal("returned query did not retire")
		}
	})
}

func TestSourcePresenceStopCannotHideIndependentFailure(t *testing.T) {
	for _, kind := range []string{"native", "deadline", "capacity", "closed", "mixed", "owner deadline", "projected cancellation"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if kind == "owner deadline" {
					ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
					defer cancel()
				}
				failure := authorityCheckFailed("authority-process-exited", syscall.Errno(5))
				err := watchSourcePresence(ctx, func() error {
					if kind == "owner deadline" {
						<-ctx.Done()
						return queryAdmissionFailure(sourcePresenceAuthorityQuery, "", ctx.Err())
					}
					cancel()
					switch kind {
					case "deadline":
						return queryAdmissionFailure(sourcePresenceAuthorityQuery, "", context.DeadlineExceeded)
					case "capacity", "closed":
						return queryAdmissionFailure(sourcePresenceAuthorityQuery, kind, nil)
					case "mixed":
						return errors.Join(queryAdmissionFailure(sourcePresenceAuthorityQuery, "", context.Canceled), failure)
					case "projected cancellation":
						return sourceAuthorityResult(queryAdmissionFailure(sourcePresenceAuthorityQuery, "", context.Canceled))
					default:
						return failure
					}
				})
				if err == nil || !errors.Is(err, ErrAuthority) {
					t.Fatal("stop hid independent failure", err)
				}
				if kind == "native" || kind == "mixed" {
					code, native := reportFailureDetails(err)
					if reportOperations[code] != "authority-process-exited" || native != 5 {
						t.Fatal("lost native denial", reportOperations[code], native)
					}
				}
			})
		})
	}
}
