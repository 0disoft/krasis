package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/truststore"
)

// A failed nonblocking store lock is contention, not a trust verdict. Retry only
// that exact sentinel; never retry a revoked, missing, corrupt or unreadable peer.
// Five checks add at most 100 ms of wait to the existing one-second monitor.
func checkMonitoredTrust(ctx context.Context, check func() error, wait func(context.Context, time.Duration) error) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := check()
		if err != truststore.ErrStoreBusy || attempt == 4 {
			return err
		}
		if err := wait(ctx, 25*time.Millisecond); err != nil {
			return err
		}
	}
}

func monitorProductSession(ctx context.Context, ticks <-chan time.Time, guardDone <-chan struct{}, check func() error, cancel context.CancelFunc, record func(string)) {
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				record("session_certificate_deadline")
			}
			return
		case <-guardDone:
			if ctx.Err() == nil {
				record("session_guard_exited")
				cancel()
			}
			return
		case <-ticks:
			err := checkMonitoredTrust(ctx, check, agent.Wait)
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			code := "session_trust_check_failed"
			if err == truststore.ErrStoreBusy {
				code = "session_trust_busy_exhausted"
			}
			record(code)
			cancel()
			return
		}
	}
}
