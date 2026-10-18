package productpanel

import (
	"context"
	"errors"
	"testing"
)

func TestServiceRepairIsExplicitAndNeverFallsBackToOrdinaryInstall(t *testing.T) {
	for _, state := range []string{"SERVICE_STOPPED", "SERVICE_RECOVERY_REQUIRED"} {
		t.Run(state, func(t *testing.T) {
			current, repairs := state, 0
			b := &inputRefresh{inspect: func() string { return current }, run: func(context.Context) error { t.Fatal("ordinary install bypassed repair"); return nil }, repair: func(context.Context) error { repairs++; current = "READY"; return nil }}
			if b.automatic(context.Background()) != nil || repairs != 0 {
				t.Fatal("automatic recovery attempted")
			}
			if b.explicit(context.Background()) != nil || repairs != 1 || b.status() != "READY" {
				t.Fatal("explicit recovery failed")
			}
		})
	}
	for _, outcome := range []error{ErrInputRefreshCancelled, ErrInputRefreshPending, ErrInputRefreshFailed} {
		calls := 0
		b := &inputRefresh{inspect: func() string { return "SERVICE_RECOVERY_REQUIRED" }, repair: func(context.Context) error { calls++; return outcome }}
		if !errors.Is(b.explicit(context.Background()), outcome) {
			t.Fatal("repair outcome lost")
		}
		_ = b.automatic(context.Background())
		if calls != 1 {
			t.Fatal("repair repeated without user action")
		}
	}
}
