package agent

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestBackoffJitterUsesDeterministicBoundarySamples(t *testing.T) {
	policy := DefaultBackoffPolicy()
	policy.Jitter = true
	for _, sample := range []float64{0, 0.5, math.Nextafter(1, 0)} {
		calls := 0
		delay, err := policy.delay(1, func() float64 { calls++; return sample })
		if err != nil || calls != 1 || delay < 200*time.Millisecond || delay > 300*time.Millisecond {
			t.Fatal("jitter left its exact bounded interval", sample, delay, calls, err)
		}
		if sample == 0.5 && delay != policy.Initial {
			t.Fatal("midpoint changed the base delay", delay)
		}
	}
	for _, sample := range []float64{-1, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := policy.delay(1, func() float64 { return sample }); err == nil {
			t.Fatal("invalid injected sample accepted", sample)
		}
	}
}

func TestSupervisorUsesInjectedRetryRandomness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	policy := DefaultBackoffPolicy()
	policy.Jitter = true
	calls := 0
	var waited time.Duration
	err := RunSupervisor(ctx, SupervisorConfig{
		Session: Config{QueueCapacity: 1, Now: time.Now}, Backoff: policy, Now: time.Now,
		Random: func() float64 { calls++; return 0.5 },
		Wait:   func(_ context.Context, delay time.Duration) error { waited = delay; cancel(); return ctx.Err() },
	}, SupervisorDependencies{
		Source: &scriptedSource{outcomes: []sourceOutcome{{err: errors.New("offline")}}},
		Hook:   &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		RunSession: func(context.Context, Config, Dependencies) (Result, error) {
			t.Fatal("failed acquisition started a session")
			return Result{FinalState: control.StateLocal}, nil
		},
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || waited != policy.Initial {
		t.Fatal("supervisor bypassed deterministic retry dependency", calls, waited, err)
	}
}

func TestBackoffJitterSaturatesBeforeDurationConversion(t *testing.T) {
	for _, maximum := range []time.Duration{1, 2, time.Second, time.Duration(math.MaxInt64 - 1), time.Duration(math.MaxInt64)} {
		policy := BackoffPolicy{Jitter: true, Initial: maximum, Maximum: maximum, StableAfter: time.Second}
		for _, sample := range []float64{0.5, math.Nextafter(1, 0)} {
			delay, err := policy.delay(math.MaxUint32, func() float64 { return sample })
			if err != nil || delay != maximum {
				t.Fatal("capped retry overflowed into a near-zero delay", maximum, sample, delay, err)
			}
		}
	}
}

func TestBackoffValidationAndNonJitterNeverConsumeRandomness(t *testing.T) {
	for _, attempt := range []uint32{0, 1, math.MaxUint32} {
		policy := DefaultBackoffPolicy()
		delay, err := policy.delay(attempt, func() float64 { t.Fatal("unneeded random call"); return 0 })
		if attempt == 0 && err == nil || attempt != 0 && (err != nil || delay < 1 || delay > policy.Maximum) {
			t.Fatal("attempt boundary changed", attempt, delay, err)
		}
	}
}
