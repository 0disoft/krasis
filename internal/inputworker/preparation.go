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
