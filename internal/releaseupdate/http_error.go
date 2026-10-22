package releaseupdate

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxRetryAfter = 5 * time.Minute

// HTTPError preserves bounded retry metadata without retaining response bodies
// or asset URLs. It describes a failed GET, never permission to replay a write.
type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("release download HTTP status %d", e.StatusCode)
}

func retryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		if seconds >= int64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return min(maxRetryAfter, max(0, date.Sub(now)))
	}
	return 0
}
