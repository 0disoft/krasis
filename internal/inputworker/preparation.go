package inputworker

import "time"

// Preparation deliberately has no identity, binding, expiry extension or input.
// It uses the existing fixed-size authenticated agent request envelope.
func encodePreparation() (data [leaseSize]byte) {
	copy(data[:4], "KRP1")
	data[4] = 1
	return
}

func isPreparation(data [leaseSize]byte) bool {
	return string(data[:4]) == "KRP1" && data[4] == 1 && zeroBytes(data[5:])
}

func preparationAllowed(last, now time.Time) bool {
	return !now.IsZero() && (last.IsZero() || now.Sub(last) >= 2*time.Second)
}

// Only a new authenticated hint after the unchanged launch cooldown may replace
// a nearly expired unbound child. The original child's timer is never renewed.
// A recent reentry preparation remains usable instead of being rebuilt again.
func preparationRefreshDue(last, preparedAt, now time.Time) bool {
	if preparedAt.IsZero() || !preparationAllowed(last, now) {
		return false
	}
	return now.Sub(preparedAt) >= preparedConsoleWindow*3/4
}
