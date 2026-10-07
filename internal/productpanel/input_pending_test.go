package productpanel

import (
	"context"
	"testing"
	"time"
)

func TestPendingInstallationCanCompleteWithoutAnotherConsent(t *testing.T) {
	for _, final := range []string{"READY", "UPDATE_REQUIRED", "UNKNOWN"} {
		state, starts := "PENDING", 0
		b := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error { starts++; return nil }}
		if b.automatic(context.Background()) != ErrInputRefreshPending {
			t.Fatal("unfinished transaction was not fenced")
		}
		state = final
		b.mu.Lock()
		b.checkedAt = time.Now().Add(-2 * time.Second)
		b.mu.Unlock()
		b.status() // Completion observation is asynchronous.
		b.mu.Lock()
		b.mu.Unlock()
		if b.status() != final {
			t.Fatal("installation outcome remained stuck pending")
		}
		_ = b.automatic(context.Background())
		if starts != 0 {
			t.Fatal("completion triggered another automatic installer")
		}
	}
}
