package inputdesktop

import (
	"context"
	"errors"
	"time"
)

const desktopPreparationBudget = 100 * time.Millisecond
const desktopPreparationInterval = 10 * time.Millisecond

// Private marker: only fixed interactive desktop acquisition/activity can
// request another pre-dispatch check. Authorization, cleanup and input errors
// cannot opt into this wait. There is no operation callback in this layer.
var errDesktopPreparing = errors.New("fixed input desktop is transitioning")

func desktopPreparing(err error) error {
	if err == nil || errors.Is(err, ErrCleanup) {
		return err
	}
	return errors.Join(errDesktopPreparing, err)
}

func (b *interactiveBinding) activeContext(ctx, root context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := b.active()
	if !errors.Is(err, errDesktopPreparing) {
		return err
	}
	// Fast successful operations allocate no timer/context. A transition wait
	// shares the request's original deadline and immutable lease expiration.
	wait, cancel := context.WithTimeout(ctx, desktopPreparationBudget)
	defer cancel()
	stop := context.AfterFunc(root, cancel)
	defer stop()
	for {
		timer := time.NewTimer(desktopPreparationInterval)
		select {
		case <-wait.Done():
			timer.Stop()
			return errors.Join(err, wait.Err())
		case <-timer.C:
		}
		if err := wait.Err(); err != nil {
			return err
		}
		err = b.active() // Reauthorizes and retires stale handles before acquisition.
		if !errors.Is(err, errDesktopPreparing) {
			return err
		}
	}
}
