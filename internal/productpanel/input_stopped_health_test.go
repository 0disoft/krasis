package productpanel

import (
	"context"
	"testing"
)

func TestStoppedServiceOffersOnlyExplicitVerifiedRepair(t *testing.T) {
	state, calls := "SERVICE_STOPPED", 0
	b := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error { calls++; state = "READY"; return nil }}
	if err := b.automatic(context.Background()); err != nil || calls != 0 {
		t.Fatal("health observation silently repaired service", err)
	}
	if err := b.explicit(context.Background()); err != nil || calls != 1 || b.status() != "READY" {
		t.Fatal("explicit stopped-service repair unavailable", err, calls)
	}
	for _, unconfirmed := range []string{"UNKNOWN", "SERVICE_RECOVERY_REQUIRED"} {
		state = unconfirmed
		if err := b.automatic(context.Background()); err != nil || calls != 1 {
			t.Fatal("unconfirmed cleanup silently repaired", err, calls)
		}
		if err := b.explicit(context.Background()); err == nil || calls != 1 {
			t.Fatal("unconfirmed identity/cleanup allowed repair", err, calls)
		}
	}
}
