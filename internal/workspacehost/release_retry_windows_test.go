//go:build windows && desktop

package workspacehost

import (
	"testing"
	"time"
)

func TestReleaseRetryBudgetReturnsToNormalCadence(t *testing.T) {
	for i, want := range []time.Duration{6 * time.Hour, time.Minute, 5 * time.Minute, 15 * time.Minute, 6 * time.Hour, 6 * time.Hour} {
		if releaseCheckDelay(i) != want {
			t.Fatal("unbounded or excessive release retry", i)
		}
	}
}
