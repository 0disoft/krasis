//go:build windows

package inputworker

import (
	"context"
	"github.com/0disoft/krasis/internal/wininput"
)

func startAuthorityQuery(ctx context.Context, kind authorityQueryKind, work func()) {
	// Capacity and the per-channel query owner are acquired before this call.
	// A timed-out native query keeps both until it really completes, just as
	// before. Do not promote speculative preparation or high-rate input writes.
	critical := kind == sourceInputAuthorityQuery || kind == sourcePresenceAuthorityQuery || ctx.Value(sourceCommandAuthorityKey{}) == true
	go func() {
		if critical {
			wininput.RunSourceInputTask(work)
			return
		}
		work()
	}()
}
