package inputworker

import (
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

const preparedConsoleWindow = 2 * time.Second

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
