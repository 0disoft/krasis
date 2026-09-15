package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type disconnectTestConnection struct {
	agent.Connection
	frames  []transport.Frame
	closed  int
	blocked bool
}

func (c *disconnectTestConnection) WriteFrameContext(ctx context.Context, frame transport.Frame) error {
	if c.blocked {
		<-ctx.Done()
		return ctx.Err()
	}
	c.frames = append(c.frames, frame)
	return nil
}
func (c *disconnectTestConnection) Close() error { c.closed++; return nil }

func TestDisconnectNotificationIsIntentionalBoundedAndOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, cause := range []error{errLocalDisconnect, context.Canceled, errors.New("trust expired")} {
			for _, blocked := range []bool{false, true} {
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(cause)
				raw := &disconnectTestConnection{blocked: blocked}
				connection := &disconnectConnection{Connection: raw, intent: ctx}
				start := time.Now()
				// Mutex waits are not durably blocked in synctest. Exercise
				// the fake deadline without competing Once callers, then
				// verify concurrent/idempotent close independently below.
				if blocked {
					if err := connection.Close(); err != nil {
						t.Fatal(err)
					}
				}
				var workers sync.WaitGroup
				for range 3 {
					workers.Go(func() {
						if err := connection.Close(); err != nil {
							t.Error(err)
						}
					})
				}
				workers.Wait()
				if raw.closed != 1 || time.Since(start) > 500*time.Millisecond {
					t.Fatalf("close=%d duration=%s", raw.closed, time.Since(start))
				}
				want := 0
				if cause == errLocalDisconnect && !blocked {
					want = 1
				}
				if len(raw.frames) != want {
					t.Fatalf("cause=%v frames=%v", cause, raw.frames)
				}
				if want == 1 && (raw.frames[0].Kind != transport.MessageDisconnect || len(raw.frames[0].Payload) != 0) {
					t.Fatal("invalid stop frame")
				}
			}
		}
	})
}

func TestRemoteDisconnectPublishesLocalAndPreservesCleanupErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, cleanupErr := range []error{nil, context.DeadlineExceeded, errors.New("release failed")} {
			operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
				return errors.Join(peer.ErrPeerDisconnectRequested, cleanupErr)
			})
			if _, err := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			status, _ := operations.Status(context.Background())
			if status.State != "LOCAL" {
				t.Fatalf("state=%s", status.State)
			}
			want := "session_stopped"
			if cleanupErr != nil {
				want = "session_failed"
			}
			if operations.lastCode != want {
				t.Fatalf("outcome=%s want=%s", operations.lastCode, want)
			}
		}
	})
}
