package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type disconnectTestConnection struct {
	agent.Connection
	frames       []transport.Frame
	closed       int
	blocked      bool
	readProgress func(int, time.Time)
}

func (c *disconnectTestConnection) SetReadProgressObserver(observer func(int, time.Time)) {
	c.readProgress = observer
}

func TestDisconnectConnectionForwardsReadProgressObserver(t *testing.T) {
	raw := &disconnectTestConnection{}
	connection := &disconnectConnection{Connection: raw}
	var observed int
	connection.SetReadProgressObserver(func(count int, _ time.Time) { observed += count })
	if raw.readProgress == nil {
		t.Fatal("read progress observer was not forwarded")
	}
	raw.readProgress(3, time.Now())
	if observed != 3 {
		t.Fatalf("read progress=%d", observed)
	}
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

func TestProtectedInputNotifiesPeerBeforeClosingTransport(t *testing.T) {
	for _, test := range []struct {
		cause error
		want  int
	}{
		{wininput.ErrProtectedInput, 1},
		{context.Canceled, 0},
	} {
		protected, cancel := context.WithCancelCause(context.Background())
		cancel(test.cause)
		raw := &disconnectTestConnection{}
		connection := &disconnectConnection{Connection: raw, intent: context.Background(), protected: protected}
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
		if raw.closed != 1 || len(raw.frames) != test.want {
			t.Fatalf("cause=%v closed=%d frames=%d", test.cause, raw.closed, len(raw.frames))
		}
		if test.want == 1 && raw.frames[0].Kind != transport.MessageDisconnect {
			t.Fatalf("protected stop frame=%v", raw.frames[0].Kind)
		}
	}
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
			wantState := "LOCAL"
			if cleanupErr != nil {
				wantState = "FAULTED"
			}
			if status.State != wantState {
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
