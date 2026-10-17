package inputworker

import (
	"context"
	"time"
)

// Activation's protected command desktop reauthorizes it immediately before
// native work. Frame admission still checks the pinned agent/service presence;
// other commands retain their existing read-side authorization.
func sourceCommandReadAuthorization(op byte, full, present func() error) error {
	if op == sourceHeartbeat || op == sourceActivate {
		return present()
	}
	return full()
}

// Scope includes mandatory live authorization and desktop selection. Apply
// includes this same operation's native work and existing boundary guards.
// Timings are observations only; they cannot grant authority or change outcomes.
func executeSourceCommand(ctx context.Context, op byte, desktop executor, work func() error) (timing [3]byte, result error) {
	if op == sourceDeactivate || op == sourceHeartbeat || op == sourceCursorRestore {
		return timing, work()
	}
	if op != sourceActivate {
		return timing, desktop(ctx, work)
	}
	started := time.Now()
	var applied time.Duration
	result = desktop(ctx, func() error {
		begin := time.Now()
		err := work()
		applied = time.Since(begin)
		timing[2] = timingBucket(applied)
		return err
	})
	timing[1] = timingBucket(time.Since(started) - applied)
	return timing, result
}
