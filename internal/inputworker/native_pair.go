package inputworker

import (
	"context"
	"errors"
)

// Neither child can be used until the pair is ready. Failed startup retires
// every successfully created owner before the caller releases inherited pins.
func startNativePair(ctx context.Context, start func(context.Context, bool) (*isolatedInput, error)) (*isolatedInput, error) {
	if ctx == nil || ctx.Err() != nil || start == nil {
		return nil, ErrAuthority
	}
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
		for _, owner := range owners {
			if owner == nil {
				continue
			}
			if owner.retire == nil {
				failure = errors.Join(failure, ErrRecovery)
				continue
			}
			retired, err := owner.retire()
			failure = errors.Join(failure, err)
			if !retired {
				failure = errors.Join(failure, ErrRecovery)
			}
		}
		return nil, failure
	}
	input, release := owners[0], owners[1]
	input.recovery, input.recoveryRetire = release.Client, release.retire
	return input, nil
}
