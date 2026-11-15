//go:build windows

package residentagent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/wininput"
)

// Native owners and their client handles have already joined. Keep mTLS alive
// while the ordinary local-return owner revokes a still-active outbound lease.
// A real release/write failure or missing completion still fails the session.
func settleRetiredSource(ctx context.Context, session *wininput.SourceSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !session.PauseRetiredCapture() {
		return wininput.ErrProtectedInput
	}
	cleanup, cancel := context.WithTimeout(ctx, wininput.SourceCleanupTimeout)
	defer cancel()
	for session.LocalReturnPending() {
		if err := agent.Wait(cleanup, 10*time.Millisecond); err != nil {
			return errors.Join(wininput.ErrProtectedInput, err)
		}
	}
	return cleanup.Err() // Cancellation/expiry cannot publish a replacement owner.
}
