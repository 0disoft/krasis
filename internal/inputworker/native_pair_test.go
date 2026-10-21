package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestNativePairStartupOverlapsAndWaitsForBothAuthenticatedOwners(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var ready, retired atomic.Int32
		owners := [2]*isolatedInput{}
		started := time.Now()
		input, err := startNativePair(context.Background(), func(ctx context.Context, release bool) (*isolatedInput, error) {
			i := 0
			if release {
				i = 1
			}
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			owners[i] = &isolatedInput{Client: &Client{}, retire: func() (bool, error) { retired.Add(1); return true, nil }}
			ready.Add(1)
			return owners[i], nil
		})
		if err != nil || time.Since(started) != 200*time.Millisecond || ready.Load() != 2 || retired.Load() != 0 || input != owners[0] || input.recovery != owners[1].Client {
			t.Fatal("pair preparation serialized or published partial readiness", err, time.Since(started), ready.Load())
		}
		if err := input.finishRecovery(); err != nil || retired.Load() != 1 {
			t.Fatal("release owner was not retained", err)
		}
	})
}

func TestNativePairStartupFailureJoinsAndRetiresLateOwner(t *testing.T) {
	for _, failRelease := range []bool{false, true} {
		for _, confirmed := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				failure := errors.New("authentication rejected")
				var canceled atomic.Bool
				var retired atomic.Int32
				started := time.Now()
				input, err := startNativePair(context.Background(), func(ctx context.Context, release bool) (*isolatedInput, error) {
					if release == failRelease {
						time.Sleep(10 * time.Millisecond)
						return nil, failure
					}
					// Like a native launch, this owner can finish after cancellation.
					time.Sleep(100 * time.Millisecond)
					canceled.Store(ctx.Err() != nil)
					return &isolatedInput{Client: &Client{}, retire: func() (bool, error) { retired.Add(1); return confirmed, nil }}, nil
				})
				if input != nil || !errors.Is(err, failure) || !canceled.Load() || time.Since(started) != 100*time.Millisecond || retired.Load() != 1 || errors.Is(err, ErrRecovery) != !confirmed {
					t.Fatal("failed pair abandoned a launch or lost retirement uncertainty", input, err, retired.Load(), time.Since(started))
				}
			})
		}
	}
}

func TestNativePairCanceledStartupCannotPublishLateReadyPair(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		var retired atomic.Int32
		started := time.Now()
		input, err := startNativePair(ctx, func(context.Context, bool) (*isolatedInput, error) {
			time.Sleep(100 * time.Millisecond) // Native launch can ignore cancellation.
			return &isolatedInput{Client: &Client{}, retire: func() (bool, error) { retired.Add(1); return true, nil }}, nil
		})
		if input != nil || !errors.Is(err, context.DeadlineExceeded) || retired.Load() != 2 || time.Since(started) != 100*time.Millisecond {
			t.Fatal("canceled caller received late input authority or lost an owner", input, err, retired.Load())
		}
	})
}
