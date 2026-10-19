package inputworker

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPreparationFreshHintReplacesNearExpiryButNeverRecentOrRateLimitedChild(t *testing.T) {
	now := time.Unix(100, 0)
	for _, test := range []struct {
		name                  string
		last, preparedAt, now time.Time
		want                  bool
	}{
		{"fresh hint before the next entry", now.Add(-2100 * time.Millisecond), now.Add(-1950 * time.Millisecond), now, true},
		{"recent reentry preparation", now.Add(-time.Minute), now.Add(-100 * time.Millisecond), now, false},
		{"less than half aged", now.Add(-time.Minute), now.Add(-time.Second + time.Nanosecond), now, false},
		{"half aged", now.Add(-time.Minute), now.Add(-time.Second), now, true},
		{"rate limited", now.Add(-time.Second), now.Add(-1900 * time.Millisecond), now, false},
		{"already expired", now.Add(-3 * time.Second), now.Add(-2500 * time.Millisecond), now, true},
		{"future preparation", now.Add(-time.Minute), now.Add(time.Second), now, false},
		{"missing preparation", now.Add(-time.Minute), time.Time{}, now, false},
		{"missing clock", now.Add(-time.Minute), now.Add(-time.Second), time.Time{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := preparationRefreshDue(test.last, test.preparedAt, test.now); got != test.want {
				t.Fatal("freshness or launch bound changed", got, test.want)
			}
		})
	}
}

// Replay the ordering seen in g345: clean return prepares a child, a fresh
// movement hint arrives about 1.1s later, and entry is still 1.58s away. Keeping
// that old child acknowledges a hint but leaves entry on the cold-launch path.
func TestPreparationFreshHintReplacesAgingReentryBeforeSlowApproach(t *testing.T) {
	preparedAt := time.Unix(100, 0)
	lastHintLaunch := preparedAt.Add(-time.Minute)
	hintAt := preparedAt.Add(1100 * time.Millisecond)
	entryAt := hintAt.Add(1580 * time.Millisecond)
	if preparedConsoleHealth(entryAt.Sub(preparedAt), 1, 1, true) != handoff.PreparationExpired {
		t.Fatal("fixture must reproduce expiration before entry")
	}
	if !preparationRefreshDue(lastHintLaunch, preparedAt, hintAt) {
		t.Fatal("fresh allowed hint retained child that expires during approach")
	}
	// Conservatively treat replacement as ready immediately. Real construction
	// starts its independent lifetime later, never extending the original child.
	if preparedConsoleHealth(entryAt.Sub(hintAt), 1, 1, true) != handoff.PreparationUsed {
		t.Fatal("replacement would not cover this approach")
	}
	if preparationRefreshDue(hintAt, preparedAt, hintAt.Add(time.Millisecond)) {
		t.Fatal("replacement bypassed launch cooldown")
	}
}
