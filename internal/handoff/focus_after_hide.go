package handoff

import (
	"context"
	"time"
)

// Preserve the first post-hide request's native acceptance, actual owner and
// duration together. The original restore rejection is never rewritten. State
// uses FocusProfile's existing categories; acceptance alone is not restoration.
func RecordPostHideRestore(ctx context.Context, foreground uint8, accepted bool, elapsed time.Duration) {
	if foreground < 1 || foreground > 4 || elapsed < 0 {
		return
	}
	if elapsed > Window {
		elapsed = Window
	}
	updateFocus(ctx, func(p *FocusProfile) {
		if p.ResultMeasured&1 != 0 && p.Restore == 3 && !p.PostHideRestoreMeasured {
			p.PostHideRestoreForeground = foreground
			p.PostHideRestoreAccepted = accepted
			p.PostHideRestoreMicros = uint32(elapsed.Microseconds())
			p.PostHideRestoreMeasured = true
		}
	})
}
