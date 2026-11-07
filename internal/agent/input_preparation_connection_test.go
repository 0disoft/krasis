package agent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

func TestConnectionPreparationStartsBeforeMovementAndSharesCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		hints := make(chan time.Time, 2)
		port := preparationTestPort{calls: make(chan struct{}, 3)}
		done := make(chan error, 1)
		go func() {
			done <- runPreparationReceiver(ctx, hints, runtime, port, transport.AuthenticatedPeerContext{}, true)
		}()
		synctest.Wait()
		if len(port.calls) != 1 || runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatal("connection did not prepare once before physical movement, or granted input")
		}
		hints <- time.Now()
		synctest.Wait()
		if len(port.calls) != 1 {
			t.Fatal("initial demand bypassed movement cooldown")
		}
		time.Sleep(2*time.Second - time.Nanosecond)
		hints <- time.Now()
		synctest.Wait()
		if len(port.calls) != 1 {
			t.Fatal("hint before cooldown boundary started another preparation")
		}
		time.Sleep(time.Nanosecond)
		hints <- time.Now()
		synctest.Wait()
		if len(port.calls) != 2 {
			t.Fatal("fresh demand at cooldown boundary was lost")
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if len(port.calls) != 2 {
			t.Fatal("idle connection periodically renewed preparation")
		}
		cancel()
		if !errors.Is(<-done, context.Canceled) {
			t.Fatal("connection preparation did not join cancellation")
		}
	})
}

func TestConnectionPreparationRetainsCancellationAndBudget(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "cancelled"}[cancelled], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				port := budgetPreparationPort{duration: time.Second, done: make(chan error, 1)}
				if cancelled {
					cancel()
				}
				done := make(chan error, 1)
				started := time.Now()
				go func() {
					done <- runPreparationReceiver(ctx, nil, runtime, port, transport.AuthenticatedPeerContext{}, true)
				}()
				if !cancelled {
					if err := <-port.done; !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != InputPreparationTimeout {
						t.Fatal("initial preparation escaped its existing budget", err, time.Since(started))
					}
					cancel()
				}
				if !errors.Is(<-done, context.Canceled) || len(port.done) != 0 || runtime.Snapshot().Control.State != control.StateLocal {
					t.Fatal("cancelled demand performed work, retained an owner, or granted input")
				}
			})
		})
	}
}

func TestNegotiatedConnectionPreparesWithoutBoundaryMovement(t *testing.T) {
	left, right := openConnectionPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	config := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 1000, RearmDistance: 32})
	port := preparationTestPort{calls: make(chan struct{}, 2)}
	results := make(chan Result, 2)
	for i, connection := range []Connection{left, right} {
		go func() {
			var authorizer peer.LeaseAuthorizer = acceptingAuthorizer{}
			if i == 1 {
				authorizer = preparationAuthorizer{preparationTestPort: port}
			}
			result, _ := RunSession(ctx, config, Dependencies{Connection: &supervisedConnection{Connection: preparationConnection{connection}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: authorizer})
			results <- result
		}()
	}
	select {
	case <-port.calls:
	case <-ctx.Done():
		t.Fatal("first preparation still waited for boundary movement")
	}
	cancel()
	for range 2 {
		select {
		case result := <-results:
			if result.FinalState != control.StateLocal {
				t.Fatal("initial preparation granted remote input")
			}
		case <-time.After(time.Second):
			t.Fatal("initial preparation blocked session retirement")
		}
	}
}
