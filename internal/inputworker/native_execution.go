package inputworker

import (
	"context"
	"time"
)

// desktop must be the native child's authenticated Scope.Do. Its binding
// freshly checks the agent/session; do not repeat that RPC before entering it.
// Service authorization precedes desktop selection, so a slow SCM query cannot
// leave the selected desktop stale. Parent/lease checks precede native work.
func nativeInputExecutor(desktop executor, service func() error, parent func() bool, expires time.Time, releaseOnly bool) executor {
	return func(ctx context.Context, work func() error) error {
		if ctx == nil || work == nil || desktop == nil || service == nil || parent == nil {
			return ErrAuthority
		}
		valid := func() bool { return releaseOnly || time.Now().Before(expires) }
		if ctx.Err() != nil || !valid() {
			return ErrAuthority
		}
		if err := service(); err != nil {
			return err
		}
		return desktop(ctx, func() error {
			if !parent() || !valid() {
				return ErrAuthority
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			return work()
		})
	}
}
