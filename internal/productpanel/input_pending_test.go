package productpanel

import (
	"context"
	"testing"
)

func TestPendingInstallationCanCompleteWithoutAnotherConsent(t *testing.T) {
	for _, final := range []string{"READY", "UPDATE_REQUIRED", "UNKNOWN"} {
		state, starts := "PENDING", 0
		b := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error { starts++; return nil }}
		if b.automatic(context.Background()) != ErrInputRefreshPending {
			t.Fatal("unfinished transaction was not fenced")
		}
		state = final
		if b.status() != final {
			t.Fatal("installation outcome remained stuck pending")
		}
		_ = b.automatic(context.Background())
		if starts != 0 {
			t.Fatal("completion triggered another automatic installer")
		}
	}
}
