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

type foregroundLossHook struct{ cleanup error }

func (hook foregroundLossHook) RunWithBoundary(ctx context.Context, session *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
	ready()
	session.Abort(wininput.ErrSourceCaptureLostForeground)
	<-ctx.Done()
	return errors.Join(ctx.Err(), wininput.ErrSourceCaptureLostForeground, hook.cleanup)
}

func TestSessionReconnectsOnlyAfterVerifiedForegroundLossCleanup(t *testing.T) {
	for _, test := range []struct {
		name    string
		cleanup error
	}{
		{name: "native-owner-released"},
		{name: "native-cleanup-failed", cleanup: errors.New("native cleanup failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			left, right := openConnectionPair(t)
			defer left.Close()
			defer right.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{
				Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10,
			}), Dependencies{
				Connection: right, Hook: foregroundLossHook{cleanup: test.cleanup},
				Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
			})
			if !errors.Is(err, wininput.ErrSourceCaptureLostForeground) {
				t.Fatalf("foreground loss omitted: %v", err)
			}
			if result.FinalState != control.StateLocal {
				t.Fatalf("input did not return LOCAL: %+v, %v", result, err)
			}
			if test.cleanup == nil {
				if errors.Is(err, ErrRecoveryUnverified) || result.Recovery.NativeOwnersReleased != RecoveryConfirmed || result.Recovery.PressedLedgerEmpty != RecoveryConfirmed {
					t.Fatalf("clean foreground loss cannot reconnect: %+v, %v", result.Recovery, err)
				}
			} else if !errors.Is(err, test.cleanup) || !errors.Is(err, ErrRecoveryUnverified) {
				t.Fatalf("native cleanup failure allowed reconnect: %v", err)
			}
		})
	}
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
