package productpanel

import (
	"context"
	"errors"
	"time"
)

const diagnosticWaitLimit = 6 * time.Second

var errDiagnosticUnavailable = errors.New("diagnostic export unavailable or still running")

// A stalled filesystem operation may outlive cancellation, but at most one
// exporter belongs to this lifecycle. It cannot mutate lifecycle/UI state.
func (b *DesktopLifecycle) diagnoseBounded(parent context.Context, fingerprint string) error {
	if b.diagnose == nil || !b.diagnosticActive.CompareAndSwap(false, true) {
		return errDiagnosticUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, diagnosticWaitLimit)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := b.diagnose(ctx, fingerprint)
		b.diagnosticActive.Store(false)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func withDiagnosticNotice(value BridgeSnapshot, err error) BridgeSnapshot {
	// Keep an authoritative transport, trust or safety failure more prominent.
	if err != nil && value.Code == "ok" {
		value.Code = "diagnostic_failed"
	}
	return value
}
