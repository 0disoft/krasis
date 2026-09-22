package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/wininput"
	"sync/atomic"
)

// A geometry belongs to one product session. Returning to the old resolution
// does not revive an invalidated session; a fresh runner must construct it again.
func displayGeometryGuard(ctx context.Context, expected wininput.DisplayGeometry, read func() wininput.DisplayGeometry, stop func()) func() error {
	var invalid atomic.Bool
	return func() error {
		if invalid.Load() {
			return wininput.ErrDisplayGeometryChanged
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !expected.Valid() || read() != expected {
			if invalid.CompareAndSwap(false, true) {
				stop()
			}
			return wininput.ErrDisplayGeometryChanged
		}
		return nil
	}
}
