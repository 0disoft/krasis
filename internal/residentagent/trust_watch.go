package residentagent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func monitorProductSessionObserved(ctx context.Context, ticks <-chan time.Time, guardDone <-chan struct{}, check func(*truststore.ReadTrace) error, cancel context.CancelFunc, record func(string), recordTrace func(truststore.ReadTraceSnapshot)) {
	deadline := time.Now().Add(trustValidationMaxAge)
	readCtx, stopReader := context.WithCancel(ctx)
	defer stopReader()
	reader := startSessionTrustReader(readCtx, ticks, deadline, check, recordTrace != nil)
	freshness := time.NewTimer(time.Until(deadline))
	defer freshness.Stop()
	soft := time.NewTimer(trustReadTimeout)
	soft.Stop()
	defer soft.Stop()
	var softDone <-chan time.Time
	var observedRead time.Time
	var seen uint64
	deferred := false
	markDeferred := func() {
		if !deferred {
			record("session_trust_read_deferred")
			deferred = true
		}
	}
	expire := func(current *trustReadObservation) {
		if ctx.Err() != nil {
			return
		}
		if recordTrace != nil {
			var pending <-chan trustCheckCompletion
			var trace *truststore.ReadTrace
			if current != nil && current.reading {
				pending, trace = make(chan trustCheckCompletion), current.trace
			}
			recordTrustExpiry(deadline, pending, trace, record)
			if trace != nil {
				recordTrace(trace.Snapshot())
			}
		}
		if ctx.Err() == nil {
			record("session_trust_freshness_expired")
			cancel()
		}
	}
	reobserve := false
	for {
		softObservation := false
		if !reobserve {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					record("session_certificate_deadline")
				}
				return
			case <-guardDone:
				if ctx.Err() == nil {
					record("session_guard_exited")
					cancel()
				}
				return
			case <-freshness.C:
			case <-reader.wake:
			case <-softDone:
				softDone = nil
				softObservation = true
			}
		}
		reobserve = false
		if ctx.Err() != nil {
			continue
		}
		// Read the latest independently validated completion before the old
		// watchdog. Observer stalls cannot discard a continuous timely chain.
		current := reader.latest.Load()
		if current != nil && !current.validatedAt.IsZero() {
			validatedDeadline := current.validatedAt.Add(trustValidationMaxAge)
			if validatedDeadline.After(deadline) {
				deadline = validatedDeadline
				freshness.Reset(max(0, time.Until(deadline)))
			}
		}
		if current != nil && current.serial > seen {
			seen = current.serial
			completed := current.completed
			err := completed.err
			if err == nil {
				if !time.Now().Before(completed.finished.Add(trustValidationMaxAge)) {
					if reader.latest.Load() != current {
						reobserve = true
						continue
					}
					record("session_trust_freshness_expired")
					cancel()
					return
				}
				deadline = completed.finished.Add(trustValidationMaxAge)
				freshness.Reset(time.Until(deadline))
				if deferred {
					record("session_trust_read_recovered")
					deferred = false
				}
			} else if deferredTrustRead(err) {
				markDeferred()
			} else {
				if recordTrace != nil && current.completedTrace != nil {
					recordTrace(current.completedTrace.Snapshot())
				}
				code := "session_trust_check_failed"
				switch {
				case errors.Is(err, context.DeadlineExceeded):
					code = "session_trust_freshness_expired"
				case errors.Is(err, truststore.ErrPeerRevoked):
					code = "session_trust_revoked"
				case errors.Is(err, truststore.ErrPeerNotFound):
					code = "session_trust_peer_missing"
				case errors.Is(err, truststore.ErrStoreNotFound):
					code = "session_trust_store_missing"
				case errors.Is(err, errMonitorGuardExited):
					code = "session_guard_exited"
				}
				record(code)
				cancel()
				return
			}
			if recordTrace != nil && current.completedTrace != nil {
				snapshot := current.completedTrace.Snapshot()
				if snapshot.Elapsed >= 50*time.Millisecond {
					recordTrace(snapshot)
				}
			}
		}
		if !time.Now().Before(deadline) {
			// Diagnostic callbacks or scheduling can stall after the snapshot
			// above. Reobserve an independently completed validation before
			// expiring the old snapshot. No deadline is renewed from delivery.
			if reader.latest.Load() != current {
				reobserve = true
				continue
			}
			expire(current)
			return
		}
		if current == nil {
			continue
		}
		if current.deferred {
			markDeferred()
		}
		if !current.reading {
			softDone = nil
			soft.Stop()
			continue
		}
		if !current.started.Equal(observedRead) {
			observedRead = current.started
			soft.Reset(max(0, time.Until(current.started.Add(trustReadTimeout))))
			softDone = soft.C
		} else if softObservation {
			if recordTrace != nil && current.trace != nil {
				recordTrace(current.trace.Snapshot())
			}
			markDeferred()
		}
	}
}
