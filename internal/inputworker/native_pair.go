package inputworker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Neither child can be used until the pair is ready. Failed startup retires
// every successfully created owner before the caller releases inherited pins.
func startNativePair(ctx context.Context, start func(context.Context, bool) (*isolatedInput, error), profile *handoff.StartupProfile) (*isolatedInput, error) {
	if ctx == nil || ctx.Err() != nil || start == nil {
		return nil, ErrAuthority
	}
	started := time.Now()
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	type startupResult struct {
		release bool
		owner   *isolatedInput
		err     error
	}
	results := make(chan startupResult, 2)
	for _, release := range []bool{false, true} {
		go func() {
			owner, err := start(run, release)
			results <- startupResult{release, owner, err}
		}()
	}
	var owners [2]*isolatedInput
	var failure error
	for range owners {
		result := <-results // Join native launches even after cancellation.
		i := 0
		if result.release {
			i = 1
		}
		owners[i] = result.owner
		if result.err == nil && (result.owner == nil || result.owner.Client == nil || result.owner.retire == nil) {
			result.err = ErrAuthority
		}
		if result.err != nil {
			failure = errors.Join(failure, result.err)
			cancel()
		}
	}
	failure = errors.Join(failure, ctx.Err())
	if failure != nil {
		// No journal or input authority was published. These two independent
		// startup owners can retire together; still join both exact receipts
		// before the caller releases inherited pins or starts a replacement.
		var retirement sync.WaitGroup
		var failures [2]error
		for i, owner := range owners {
			if owner == nil {
				continue
			}
			if owner.retire == nil {
				failures[i] = ErrRecovery
				continue
			}
			retirement.Add(1)
			go func() {
				defer retirement.Done()
				retired, err := owner.retire()
				failures[i] = err
				if !retired {
					failures[i] = errors.Join(err, ErrRecovery)
				}
			}()
		}
		retirement.Wait()
		return nil, errors.Join(failure, failures[0], failures[1])
	}
	input, release := owners[0], owners[1]
	input.recovery, input.recoveryRetire = release.Client, release.retire
	// Both independently authenticated native owners reached readiness. Reuse
	// the desktop-ready span that the direct path already publishes; this is
	// elapsed pair startup, never the sum of concurrent child measurements.
	profile.Record(handoff.WorkerDesktopReady, time.Since(started))
	return input, nil
}
