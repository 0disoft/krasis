package inputworker

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPreparationHealthReportsTheFirstRejectedExistingCondition(t *testing.T) {
	for _, test := range []struct {
		age              time.Duration
		session, current uint32
		alive            bool
		want             handoff.PreparationDecision
	}{
		{time.Millisecond, 1, 1, true, handoff.PreparationUsed},
		{-1, 1, 1, true, handoff.PreparationInvalidAge},
		{preparedConsoleWindow, 1, 1, true, handoff.PreparationExpired},
		{preparedConsoleWindow, 1, 2, false, handoff.PreparationExpired},
		{time.Millisecond, 1, 2, true, handoff.PreparationSessionChanged},
		{time.Millisecond, 0, 0, true, handoff.PreparationSessionChanged},
		{time.Millisecond, 0xffffffff, 0xffffffff, true, handoff.PreparationSessionChanged},
		{time.Millisecond, 1, 1, false, handoff.PreparationExited},
	} {
		if got := preparedConsoleHealth(test.age, test.session, test.current, test.alive); got != test.want {
			t.Fatal("freshness decision changed or lost the rejected condition", got, test.want)
		}
	}
}
