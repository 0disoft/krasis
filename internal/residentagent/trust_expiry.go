package residentagent

import (
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

type trustCheckCompletion struct {
	err      error
	finished time.Time
}

// Terminal observation only. A queued result is never reused to renew authority
// after the freshness deadline, and this probe never waits for the worker.
func recordTrustExpiry(deadline time.Time, result <-chan trustCheckCompletion, trace *truststore.ReadTrace, record func(string)) {
	state := "idle"
	if result != nil {
		state = "pending"
		select {
		case completed := <-result:
			state = "result_ok"
			if completed.err != nil {
				state = "result_error"
			}
			if completed.finished.Before(deadline) {
				state += "_before_deadline"
			} else {
				state += "_late"
			}
		default:
			if trace != nil {
				snapshot := trace.Snapshot()
				switch {
				case snapshot.Phase != "":
					state = "reading"
				case snapshot.Attempts == 0:
					state = "not_started"
				default:
					state = "finished_unpublished"
				}
			}
		}
	}
	record("trust_expiry_" + state)
	bucket := "lt1ms"
	switch late := time.Since(deadline); {
	case late >= time.Second:
		bucket = "ge1s"
	case late >= 100*time.Millisecond:
		bucket = "lt1s"
	case late >= 10*time.Millisecond:
		bucket = "lt100ms"
	case late >= time.Millisecond:
		bucket = "lt10ms"
	}
	record("trust_expiry_late_" + bucket)
}
