package agent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
	"testing"
	"time"
)

type lateCleanupHook struct {
	ready   chan struct{}
	failure error
}

func (h *lateCleanupHook) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
	ready()
	close(h.ready)
	<-ctx.Done()
	return errors.Join(ctx.Err(), h.failure)
}

func TestSessionPreservesHookCleanupAfterStop(t *testing.T) {
	for _, remoteStop := range []bool{false, true} {
		t.Run(map[bool]string{false: "local_cancel", true: "peer_disconnect"}[remoteStop], func(t *testing.T) {
			left, right := openConnectionPair(t)
			defer left.Close()
			defer right.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			failure := errors.New("hook cleanup failed")
			hook := &lateCleanupHook{ready: make(chan struct{}), failure: failure}
			done := make(chan error, 1)
			go func() {
				_, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10}), Dependencies{Connection: right, Hook: hook, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
				done <- err
			}()
			select {
			case <-hook.ready:
			case <-ctx.Done():
				t.Fatal("hook not ready")
			}
			expected := error(context.Canceled)
			if remoteStop {
				expected = peer.ErrPeerDisconnectRequested
				if err := left.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageDisconnect}); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, expected) || !errors.Is(err, failure) || !errors.Is(err, ErrRecoveryUnverified) {
					t.Fatalf("lost stop or cleanup: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("session did not stop")
			}
		})
	}
}

func TestSupervisorDoesNotRetryUnverifiedRecovery(t *testing.T) {
	err := RunSupervisor(context.Background(), SupervisorConfig{Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: func(context.Context, time.Duration) error { t.Fatal("retried unverified recovery"); return nil }}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, errors.Join(errors.New("transport failed"), ErrRecoveryUnverified)
		},
	})
	if !errors.Is(err, ErrRecoveryUnverified) {
		t.Fatalf("lost recovery failure: %v", err)
	}
}

func TestSupervisorPreservesCleanupAfterParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("late cleanup failed")
	err := RunSupervisor(ctx, SupervisorConfig{Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now, Wait: func(context.Context, time.Duration) error { t.Fatal("retried canceled session"); return nil }}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			cancel()
			return Result{FinalState: control.StateLocal}, errors.Join(context.Canceled, failure)
		},
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
		t.Fatalf("lost cleanup: %v", err)
	}
}
