package wininput

import (
	"context"
	"time"
)

// CredentialUIProbeResult contains no credential, window, process, or input data.
type CredentialUIProbeResult string

const (
	CredentialUIProbeNotElevated        CredentialUIProbeResult = "not_elevated"
	CredentialUIProbeNotObserved        CredentialUIProbeResult = "dialog_not_observed"
	CredentialUIProbeSendFailed         CredentialUIProbeResult = "escape_send_failed"
	CredentialUIProbeStillOpen          CredentialUIProbeResult = "dialog_still_open"
	CredentialUIProbeDismissed          CredentialUIProbeResult = "dialog_dismissed_candidate"
	CredentialUIProbeDesktopUnavailable CredentialUIProbeResult = "desktop_unavailable_or_changed"
	CredentialUIProbeIdentityUnknown    CredentialUIProbeResult = "foreground_identity_unknown"
	CredentialUIProbeOtherForeground    CredentialUIProbeResult = "other_foreground"
	CredentialUIProbeNoForeground       CredentialUIProbeResult = "no_foreground"
	CredentialUIProbeBrokerObserved     CredentialUIProbeResult = "broker_observed"
)

func runCredentialUIProbe(ctx context.Context, elevated bool, observe func() CredentialUIProbeResult, sendEscape func(context.Context) error, wait time.Duration) CredentialUIProbeResult {
	if !elevated {
		return CredentialUIProbeNotElevated
	}
	deadline, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	lastMiss := CredentialUIProbeNotObserved
	for {
		state := observe()
		if state == CredentialUIProbeBrokerObserved {
			// Recheck immediately before injection; another foreground owner may
			// have replaced the credential dialog while the ticker was waiting.
			if observe() != CredentialUIProbeBrokerObserved {
				continue
			}
			if err := sendEscape(deadline); err != nil {
				return CredentialUIProbeSendFailed
			}
			observation, stop := context.WithTimeout(deadline, 2*time.Second)
			defer stop()
			for {
				if observe() != CredentialUIProbeBrokerObserved {
					return CredentialUIProbeDismissed
				}
				select {
				case <-observation.Done():
					return CredentialUIProbeStillOpen
				case <-ticker.C:
				}
			}
		} else {
			lastMiss = state
		}
		select {
		case <-deadline.Done():
			return lastMiss
		case <-ticker.C:
		}
	}
}
