package agent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"testing"
	"time"
)

func TestSupervisorStopsOnPeerDisconnectWithoutRetry(t *testing.T) {
	for _, cleanupFailure := range []bool{false, true} {
		cause := error(peer.ErrPeerDisconnectRequested)
		if cleanupFailure {
			cause = errors.Join(cause, errors.New("cleanup failed"))
		}
		observer := &recordingSupervisorObserver{}
		err := RunSupervisor(context.Background(), SupervisorConfig{
			Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now,
			Wait: func(context.Context, time.Duration) error { t.Fatal("manual stop retried"); return nil }, Observer: observer,
		}, SupervisorDependencies{
			Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &stubConnection{}}}},
			Hook:   &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
			RunSession: func(context.Context, Config, Dependencies) (Result, error) {
				return Result{FinalState: control.StateLocal}, cause
			},
		})
		if !errors.Is(err, cause) {
			t.Fatalf("lost outcome: %v", err)
		}
		if got := observer.states(); !equalStates(got, []SupervisorState{SupervisorAcquiring, SupervisorActive, SupervisorStopped}) {
			t.Fatalf("states=%v", got)
		}
	}
}

type disconnectFrameConnection struct {
	stubConnection
	payload []byte
}

func (c *disconnectFrameConnection) ReadFrame() (transport.Frame, error) {
	return transport.Frame{Kind: transport.MessageDisconnect, Payload: c.payload}, nil
}

func TestSupervisorRetainsReceivedStopAcrossConcurrentFailure(t *testing.T) {
	for _, payload := range [][]byte{nil, {1}} {
		cause := errors.New("concurrent worker failure")
		retry := errors.New("retry observed")
		waited := false
		err := RunSupervisor(context.Background(), SupervisorConfig{
			Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: DefaultBackoffPolicy(), Now: time.Now,
			Wait: func(context.Context, time.Duration) error { waited = true; return retry },
		}, SupervisorDependencies{
			Source: &scriptedSource{outcomes: []sourceOutcome{{connection: &disconnectFrameConnection{payload: payload}}}},
			Hook:   &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
			RunSession: func(_ context.Context, _ Config, d Dependencies) (Result, error) {
				if _, err := d.Connection.ReadFrame(); err != nil {
					t.Fatal(err)
				}
				return Result{FinalState: control.StateLocal}, cause
			},
		})
		if len(payload) == 0 {
			if waited || !errors.Is(err, cause) {
				t.Fatalf("valid stop lost: %v, waited=%v", err, waited)
			}
		} else if !waited || !errors.Is(err, retry) {
			t.Fatalf("malformed stop suppressed retry: %v", err)
		}
	}
}

// Exercise the real session workers and authenticated transport, not only a
// scripted supervisor outcome. No native hooks or input are installed.
func TestAgentReceivesDisconnectOverAuthenticatedConnection(t *testing.T) {
	left, right := openConnectionPair(t)
	defer left.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		result, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10}), Dependencies{
			Connection: right, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		})
		if result.FinalState != control.StateLocal {
			t.Errorf("final state=%v", result.FinalState)
		}
		done <- err
	}()
	if err := left.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageDisconnect}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, peer.ErrPeerDisconnectRequested) {
			t.Fatalf("disconnect cause=%v", err)
		}
	case <-ctx.Done():
		t.Fatal("disconnect hung")
	}
}
