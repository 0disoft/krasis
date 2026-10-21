package residentagent

import (
	"context"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

// Record the first positive observation once per runner. Do not re-query the
// foreground after detection: it may already have changed before cancellation.
func observedProtectedInput(inspect func() wininput.ProtectedInputReason, event func(string)) func() bool {
	var once sync.Once
	return func() bool {
		reason := inspect()
		if reason == wininput.ProtectedInputClear {
			return false
		}
		once.Do(func() {
			code := "protected_input_identity_unknown"
			switch reason {
			case wininput.ProtectedInputDesktopUnavailable:
				code = "protected_input_desktop_unavailable"
			case wininput.ProtectedInputNonDefaultDesktop:
				code = "protected_input_non_default_desktop"
			case wininput.ProtectedInputCredentialBroker:
				code = "protected_input_credential_broker"
			}
			event(code)
		})
		return true
	}
}

func monitorProtectedInput(ctx context.Context, ticks <-chan time.Time, active func(context.Context) (bool, error), stop func()) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticks:
			blocked, err := active(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return err
			}
			if blocked {
				stop()
				return nil
			}
		}
	}
}
