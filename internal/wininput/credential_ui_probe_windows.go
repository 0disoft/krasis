//go:build windows

package wininput

import (
	"context"
	"time"
)

// RunCredentialUIProbe is a one-shot, attended experiment. It sends only an
// Escape down/up pair to a foreground CredentialUIBroker on the Default desktop.
func RunCredentialUIProbe(ctx context.Context) CredentialUIProbeResult {
	process, err := processInfo()
	if err != nil || !process.Elevated {
		return CredentialUIProbeNotElevated
	}
	return runCredentialUIProbe(ctx, true, CredentialBrokerForeground, func(ctx context.Context) error {
		if !CredentialBrokerForeground() {
			return context.Canceled
		}
		err := sendInputs(ctx, []input{keyboardEvent(0x01, false, false), keyboardEvent(0x01, false, true)})
		if err != nil {
			// A partial batch must not leave Escape held.
			_ = sendInputs(context.Background(), []input{keyboardEvent(0x01, false, true)})
		}
		return err
	}, 60*time.Second)
}
