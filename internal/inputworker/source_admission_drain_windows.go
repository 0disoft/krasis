//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

// Caller owns mu. No query is replayed, no late permission is reused and no
// liveness timestamp is advanced. The next command still needs fresh authority.
func (c *sourceClient) drainAdmission(ctx context.Context) (pending bool, err error) {
	if c.pending.sequence == 0 || c.pending.op != sourceAdmit || c.admissionUntil.IsZero() {
		return false, nil
	}
	select {
	case reply := <-c.replies:
		p, err := sourceReplyResult(c.pending, reply)
		_, err = sourceAdmissionResult(p, err)
		if err != nil && !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) {
			return false, err
		}
		c.pending = sourcePacket{}
		c.admissionUntil = time.Time{}
		emitTiming(c.timing.emit, "installed_source_admission_drain_completed")
		return false, nil
	default:
	}
	select {
	case <-c.done:
		return false, ErrAuthority
	default:
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !time.Now().Before(c.admissionUntil) {
		emitTiming(c.timing.emit, "installed_source_admission_drain_expired")
		return false, ErrAuthority
	}
	return true, nil
}
