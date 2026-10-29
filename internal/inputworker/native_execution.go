package inputworker

import (
	"context"
	"time"
)

// desktop must be the native child's authenticated Scope.Do. Its binding
// freshly checks the agent/session; do not repeat that RPC before entering it.
// Service authorization precedes desktop selection, so a slow SCM query cannot
// leave the selected desktop stale. Parent/lease checks precede native work.
func nativeInputExecutor(desktop executor, service func() error, parent func() bool, expires time.Time, releaseOnly bool, timings ...*workerTiming) executor {
	return func(ctx context.Context, work func() error) error {
		if ctx == nil || work == nil || desktop == nil || service == nil || parent == nil {
			return ErrAuthority
		}
		valid := func() bool { return releaseOnly || time.Now().Before(expires) }
		if ctx.Err() != nil || !valid() {
			return ErrAuthority
		}
		checkedDesktop := func(ctx context.Context, operation func() error) error {
			return desktop(ctx, func() error {
				if !parent() || !valid() {
					return ErrAuthority
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				return operation()
			})
		}
		if len(timings) == 1 && timings[0] != nil {
			return measureExecution(service, checkedDesktop, timings[0])(ctx, work)
		}
		if err := service(); err != nil {
			return err
		}
		return checkedDesktop(ctx, work)
	}
}
