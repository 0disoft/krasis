package inputworker

import (
	"testing"
	"time"
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
