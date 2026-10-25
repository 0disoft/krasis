package inputworker

import (
	"context"
	"errors"
	"net"
	"time"
)

var unboundRetiredFrame = [8]byte{'K', 'R', 'U', '1', 1, 0, 0, 0}

// Called only after the service-authenticated, unbound worker consumes the
// reserved zero bootstrap. A real lease never enters this retirement path.
func retireUnboundNative(startup context.Context, control net.Conn, stopStartupIO func() bool, retire func() error) error {
	// The authenticated stop ends startup without binding input. Retain this
	// exact pipe through native exit and the receipt; the startup timer must
	// not close it during the independent, bounded retirement operation.
	if !stopStartupIO() {
		return errors.Join(ErrRecovery, startup.Err())
	}
	// The exact authenticated stop was fully consumed. A late startup expiry
	// cannot invalidate cleanup or its receipt once its pipe-close callback is
	// disarmed. No input binding/authority is admitted by this reserved frame.
	if err := control.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		return errors.Join(ErrRecovery, err)
	}
	if err := retire(); err != nil {
		return errors.Join(ErrRecovery, err)
	}
	if err := writeFrame(control, unboundRetiredFrame[:]); err != nil {
		return errors.Join(ErrRecovery, err)
	}
	return nil
}
