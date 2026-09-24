package residentagent

import (
	"context"
	"time"
)

func monitorProtectedInput(ctx context.Context, ticks <-chan time.Time, active func() bool, stop func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if ctx.Err() == nil && active() {
				stop()
				return
			}
		}
	}
}
