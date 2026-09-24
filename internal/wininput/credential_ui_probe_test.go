package wininput

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCredentialUIProbeRequiresElevationAndConfirmedDialog(t *testing.T) {
	called := false
	send := func(context.Context) error {
		called = true
		return nil
	}
	if got := runCredentialUIProbe(context.Background(), false, func() CredentialUIProbeResult { return CredentialUIProbeBrokerObserved }, send, time.Second); got != CredentialUIProbeNotElevated {
		t.Fatalf("non-elevated result = %s", got)
	}
	if got := runCredentialUIProbe(context.Background(), true, func() CredentialUIProbeResult { return CredentialUIProbeNotObserved }, send, 20*time.Millisecond); got != CredentialUIProbeNotObserved {
		t.Fatalf("unconfirmed dialog result = %s", got)
	}
	if called {
		t.Fatal("probe sent input without both prerequisites")
	}
}

func TestCredentialUIProbeRechecksForegroundBeforeInput(t *testing.T) {
	checks := 0
	called := false
	got := runCredentialUIProbe(context.Background(), true, func() CredentialUIProbeResult {
		checks++
		if checks == 1 {
			return CredentialUIProbeBrokerObserved
		}
		return CredentialUIProbeOtherForeground
	}, func(context.Context) error {
		called = true
		return nil
	}, 20*time.Millisecond)
	if got != CredentialUIProbeOtherForeground || called {
		t.Fatalf("lost foreground: result=%s inputSent=%t", got, called)
	}
}

func TestCredentialUIProbeReportsSendFailure(t *testing.T) {
	got := runCredentialUIProbe(context.Background(), true, func() CredentialUIProbeResult { return CredentialUIProbeBrokerObserved }, func(context.Context) error {
		return errors.New("injection failed")
	}, time.Second)
	if got != CredentialUIProbeSendFailed {
		t.Fatalf("send failure result = %s", got)
	}
}

func TestCredentialUIProbeReportsDialogDismissalCandidate(t *testing.T) {
	checks := 0
	got := runCredentialUIProbe(context.Background(), true, func() CredentialUIProbeResult {
		checks++
		if checks <= 2 {
			return CredentialUIProbeBrokerObserved
		}
		return CredentialUIProbeOtherForeground
	}, func(context.Context) error { return nil }, time.Second)
	if got != CredentialUIProbeDismissed {
		t.Fatalf("dialog dismissal result = %s", got)
	}
}

func TestCredentialUIProbeReportsDialogStillOpen(t *testing.T) {
	got := runCredentialUIProbe(context.Background(), true, func() CredentialUIProbeResult { return CredentialUIProbeBrokerObserved }, func(context.Context) error {
		return nil
	}, 20*time.Millisecond)
	if got != CredentialUIProbeStillOpen {
		t.Fatalf("dialog still open result = %s", got)
	}
}

func TestCredentialUIProbeReportsLastMissCategory(t *testing.T) {
	got := runCredentialUIProbe(context.Background(), true, func() CredentialUIProbeResult {
		return CredentialUIProbeDesktopUnavailable
	}, func(context.Context) error {
		t.Fatal("probe sent input without a confirmed broker")
		return nil
	}, 20*time.Millisecond)
	if got != CredentialUIProbeDesktopUnavailable {
		t.Fatalf("miss category = %s", got)
	}
}
