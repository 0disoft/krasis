package inputworker

import (
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Outlive the 2s launch gates and the sender's 2.1s renewal interval. A fresh
// movement hint must not invariably arrive after its last candidate expired.
const preparedConsoleWindow = 3 * time.Second
const preparedConsoleEntryReserve = 750 * time.Millisecond

// Same freshness predicate as before, with a bounded diagnostic for the first
// rejected condition. The result never skips native authentication or cleanup.
func preparedConsoleHealth(age time.Duration, session, current uint32, alive bool) handoff.PreparationDecision {
	if age < 0 {
		return handoff.PreparationInvalidAge
	}
	if age >= preparedConsoleWindow {
		return handoff.PreparationExpired
	}
	if session == 0 || session == 0xffffffff || session != current {
		return handoff.PreparationSessionChanged
	}
	if !alive {
		return handoff.PreparationExited
	}
	return handoff.PreparationUsed
}
