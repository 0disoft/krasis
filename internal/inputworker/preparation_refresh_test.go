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
		{"retain warm child across renewal", now.Add(-2257 * time.Millisecond), now.Add(-2107 * time.Millisecond), now, false},
		{"recent reentry preparation", now.Add(-time.Minute), now.Add(-100 * time.Millisecond), now, false},
		{"reserve remains", now.Add(-time.Minute), now.Add(-preparedConsoleWindow + preparedConsoleEntryReserve), now, false},
		{"reserve exhausted", now.Add(-time.Minute), now.Add(-preparedConsoleWindow + preparedConsoleEntryReserve - time.Nanosecond), now, true},
		{"rate limited", now.Add(-time.Second), now.Add(-1900 * time.Millisecond), now, false},
		{"already expired", now.Add(-4 * time.Second), now.Add(-preparedConsoleWindow), now, true},
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

// Replay g345's slow approach too: the 3s lifetime now covers this existing
// candidate through entry, without constructing a replacement on the approach.
func TestPreparationRetainsReentryAcrossSlowApproach(t *testing.T) {
	preparedAt := time.Unix(100, 0)
	lastHintLaunch := preparedAt.Add(-time.Minute)
	hintAt := preparedAt.Add(1100 * time.Millisecond)
	entryAt := hintAt.Add(1580 * time.Millisecond)
	if preparedConsoleHealth(entryAt.Sub(preparedAt), 1, 1, true) != handoff.PreparationUsed {
		t.Fatal("existing child expired during the slow approach")
	}
	if preparationRefreshDue(lastHintLaunch, preparedAt, hintAt) {
		t.Fatal("fresh hint replaced a child that covers this approach")
	}
	if preparationRefreshDue(hintAt, preparedAt, hintAt.Add(time.Millisecond)) {
		t.Fatal("duplicate hint bypassed launch cooldown")
	}
}
