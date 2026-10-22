package inputworker

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestNativePairStartupOverlapsAndWaitsForBothAuthenticatedOwners(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var ready, retired atomic.Int32
		owners := [2]*isolatedInput{}
		started := time.Now()
		var profile handoff.StartupProfile
		input, err := startNativePair(context.Background(), func(ctx context.Context, release bool) (*isolatedInput, error) {
			i := 0
			if release {
				i = 1
			}
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			owners[i] = &isolatedInput{Client: &Client{}, retire: func() (bool, error) { retired.Add(1); return true, nil }}
			ready.Add(1)
			return owners[i], nil
		}, &profile)
		if err != nil || time.Since(started) != 200*time.Millisecond || ready.Load() != 2 || retired.Load() != 0 || input != owners[0] || input.recovery != owners[1].Client {
			t.Fatal("pair preparation serialized or published partial readiness", err, time.Since(started), ready.Load())
		}
		var wire bytes.Buffer
		if err := writeStartupReady(&wire, profile); err != nil {
			t.Fatal(err)
		}
		received, err := readStartupReady(&wire)
		if err != nil || received == nil || received.Measured != 1<<handoff.WorkerDesktopReady || received.Micros[handoff.WorkerDesktopReady] != 200000 {
			t.Fatal("paired native readiness time lost in the production receipt", received, err)
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
				var profile handoff.StartupProfile
				input, err := startNativePair(context.Background(), func(ctx context.Context, release bool) (*isolatedInput, error) {
					if release == failRelease {
						time.Sleep(10 * time.Millisecond)
						return nil, failure
					}
					// Like a native launch, this owner can finish after cancellation.
					time.Sleep(100 * time.Millisecond)
					canceled.Store(ctx.Err() != nil)
					return &isolatedInput{Client: &Client{}, retire: func() (bool, error) { retired.Add(1); return confirmed, nil }}, nil
				}, &profile)
				if profile.Measured != 0 {
					t.Fatal("failed native pair published desktop readiness", profile)
				}
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
		var profile handoff.StartupProfile
		input, err := startNativePair(ctx, func(context.Context, bool) (*isolatedInput, error) {
			time.Sleep(100 * time.Millisecond) // Native launch can ignore cancellation.
			return &isolatedInput{Client: &Client{}, retire: func() (bool, error) { retired.Add(1); return true, nil }}, nil
		}, &profile)
		if profile.Measured != 0 {
			t.Fatal("cancelled native pair published desktop readiness", profile)
		}
		if input != nil || !errors.Is(err, context.DeadlineExceeded) || retired.Load() != 2 || time.Since(started) != 100*time.Millisecond {
			t.Fatal("canceled caller received late input authority or lost an owner", input, err, retired.Load())
		}
	})
}

func TestNativePairCanceledStartupRetiresBothUnboundOwnersTogether(t *testing.T) {
	for _, releaseConfirmed := range []bool{true, false} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			defer cancel()
			var retired atomic.Int32
			retirementErr := errors.New("input retirement failure")
			var profile handoff.StartupProfile
			started := time.Now()
			input, err := startNativePair(ctx, func(_ context.Context, release bool) (*isolatedInput, error) {
				time.Sleep(2 * time.Millisecond) // Both native launches finish late.
				return &isolatedInput{Client: &Client{}, retire: func() (bool, error) {
					if release {
						time.Sleep(20 * time.Millisecond)
						retired.Add(1)
						return releaseConfirmed, nil
					}
					time.Sleep(10 * time.Millisecond)
					retired.Add(1)
					return true, retirementErr
				}}, nil
			}, &profile)
			if input != nil || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, retirementErr) || errors.Is(err, ErrRecovery) != !releaseConfirmed {
				t.Fatal("parallel retirement lost cancellation or an owner's failure", input, err)
			}
			if elapsed := time.Since(started); elapsed != 22*time.Millisecond || retired.Load() != 2 || profile.Measured != 0 {
				t.Fatal("startup retirement serialized, abandoned an owner or published readiness", elapsed, retired.Load(), profile)
			}
		})
	}
}
