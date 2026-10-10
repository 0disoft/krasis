package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestRebootAcquireFailuresDoNotDelayFirstSessionReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var outcomes []sourceOutcome
	for i := 0; i < 8; i++ {
		outcomes = append(outcomes, sourceOutcome{err: errors.New("offline during reboot")})
	}
	outcomes = append(outcomes, sourceOutcome{connection: &stubConnection{}}, sourceOutcome{connection: &stubConnection{}})
	var waits []time.Duration
	err := RunSupervisor(ctx, SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now},
		Backoff: BackoffPolicy{Initial: time.Millisecond, Maximum: 32 * time.Millisecond, StableAfter: time.Minute},
		Now:     time.Now,
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == 10 {
				cancel()
				return ctx.Err()
			}
			return nil
		},
	}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: outcomes},
		Hook:   &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			return Result{FinalState: control.StateLocal}, errors.New("brief session failure")
		},
	})
	if !errors.Is(err, context.Canceled) || len(waits) != 10 {
		t.Fatal("supervisor did not complete the replay", err, waits)
	}
	if waits[7] != 32*time.Millisecond || waits[8] != time.Millisecond || waits[9] != 2*time.Millisecond {
		t.Fatal("offline attempts leaked into session backoff or brief faults reset it", waits)
	}
}
