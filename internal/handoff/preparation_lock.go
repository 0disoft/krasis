package handoff

import (
	"context"
	"time"
)

func validPreparationLock(s Summary) bool {
	if !s.PreparationLockMeasured {
		return s.PreparationLockMicros == 0
	}
	return s.Role == "receiver" && s.Preparation.Measured&(1<<OpenLock) != 0 &&
		uint64(s.PreparationLockMicros) <= uint64(s.Preparation.Micros[OpenLock]) &&
		s.PreparationLockMicros <= uint32(Window.Microseconds())
}

// RecordPreparationLock attaches only the overlap with the latest completed
// preparation lock interval. Raw timestamps stay in the resident agent. This
// lower bound overlaps OpenLock and must not be added to it.
func RecordPreparationLock(ctx context.Context, elapsed time.Duration) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || elapsed < 0 {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !exchangeActive(s, a) || a.summary.Role != "receiver" || a.summary.PreparationLockMeasured || a.summary.Preparation.Measured&(1<<OpenLock) == 0 {
		return
	}
	micros := uint32(min(elapsed, Window).Microseconds())
	if uint64(micros) > uint64(a.summary.Preparation.Micros[OpenLock]) {
		return
	}
	a.summary.PreparationLockMicros = micros
	a.summary.PreparationLockMeasured = true
}
