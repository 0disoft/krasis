package residentagent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/truststore"
)

const trustReadTimeout = time.Second
const trustValidationMaxAge = 5 * time.Second

func sessionTrustFailure(code string) error {
	switch code {
	case "session_trust_freshness_expired":
		return agent.ErrSessionTrustFreshness
	case "session_trust_revoked":
		return agent.ErrSessionTrustRevoked
	case "session_trust_peer_missing", "session_trust_store_missing":
		return agent.ErrSessionTrustMissing
	case "session_trust_check_failed":
		return agent.ErrSessionTrustCheck
	}
	return nil
}

var trustReadPhases = [...]string{
	"prelock", "prelock_parent_acl", "prelock_parent_reparse", "prelock_parent_stat",
	"prelock_parent_acl_query", "prelock_parent_acl_check", "prelock_directory_lstat", "prelock_directory_acl",
	"prelock_directory_open", "prelock_lockfile_open", "prelock_lockfile_acl",
	"lock", "store_read", "store_read_open", "store_read_acl", "store_read_bytes",
	"store_read_decode", "store_read_close", "certificate", "retry_wait",
}

// Emit fixed, coarse timing codes so exported diagnostics locate a slow phase
// without exposing peer identity, paths, certificates, or raw error text.
func recordTrustTrace(snapshot truststore.ReadTraceSnapshot, record func(string)) {
	for _, phase := range trustReadPhases {
		duration, observed := snapshot.Durations[phase]
		if !observed {
			continue
		}
		if (strings.HasPrefix(phase, "prelock_") || strings.HasPrefix(phase, "store_read_")) &&
			duration < 10*time.Millisecond && snapshot.Phase != phase {
			continue
		}
		bucket := "lt1ms"
		switch {
		case duration >= time.Second:
			bucket = "ge1s"
		case duration >= 100*time.Millisecond:
			bucket = "lt1s"
		case duration >= 10*time.Millisecond:
			bucket = "lt100ms"
		case duration >= time.Millisecond:
			bucket = "lt10ms"
		}
		record("trust_timing_" + phase + "_" + bucket)
	}
	for _, phase := range trustReadPhases {
		if snapshot.Phase == phase {
			record("trust_active_" + phase)
			break
		}
	}
}

var (
	errTrustReadTimeout   = errors.New("trust read deadline expired")
	errTrustReadPending   = errors.New("previous trust read has not completed")
	errMonitorGuardExited = errors.New("guard exited during trust check")
	trustReadSlot         = make(chan struct{}, 1)
)

// Only one file-reading worker may outlive a canceled check in this process.
// It has no input authority; a late result is discarded and cannot resume a session.
func boundedMonitoredTrust(ctx context.Context, guardDone <-chan struct{}, check func() error) error {
	return boundedMonitoredTrustObserved(ctx, guardDone, func(*truststore.ReadTrace) error { return check() }, nil)
}

func boundedMonitoredTrustObserved(ctx context.Context, guardDone <-chan struct{}, check func(*truststore.ReadTrace) error, trace *truststore.ReadTrace) error {
	select {
	case trustReadSlot <- struct{}{}:
	default:
		return errTrustReadPending
	}
	checkCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		err := func() error {
			defer func() { <-trustReadSlot }()
			return checkMonitoredTrustObserved(checkCtx, func() error { return check(trace) }, agent.Wait, trace)
		}()
		result <- err
	}()
	timer := time.NewTimer(trustReadTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-guardDone:
		return errMonitorGuardExited
	case <-timer.C:
		return errTrustReadTimeout
	case err := <-result:
		return err
	}
}

// A failed nonblocking store lock is contention, not a trust verdict. Retry only
// that exact sentinel; never retry a revoked, missing, corrupt or unreadable peer.
// Five checks add at most 100 ms of wait to the existing one-second monitor.
func checkMonitoredTrust(ctx context.Context, check func() error, wait func(context.Context, time.Duration) error) error {
	return checkMonitoredTrustObserved(ctx, check, wait, nil)
}

func checkMonitoredTrustObserved(ctx context.Context, check func() error, wait func(context.Context, time.Duration) error, trace *truststore.ReadTrace) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := check()
		if err != truststore.ErrStoreBusy || attempt == 4 {
			return err
		}
		if trace != nil {
			trace.EnterRetryWait()
		}
		if err := wait(ctx, 25*time.Millisecond); err != nil {
			return err
		}
	}
}

func monitorProductSession(ctx context.Context, ticks <-chan time.Time, guardDone <-chan struct{}, check func() error, cancel context.CancelFunc, record func(string)) {
	monitorProductSessionObserved(ctx, ticks, guardDone, func(*truststore.ReadTrace) error { return check() }, cancel, record, nil)
}

func monitorProductSessionObserved(ctx context.Context, ticks <-chan time.Time, guardDone <-chan struct{}, check func(*truststore.ReadTrace) error, cancel context.CancelFunc, record func(string), recordTrace func(truststore.ReadTraceSnapshot)) {
	deadline := time.Now().Add(trustValidationMaxAge)
	freshness := time.NewTimer(trustValidationMaxAge)
	defer freshness.Stop()
	soft := time.NewTimer(trustReadTimeout)
	soft.Stop()
	defer soft.Stop()
	var softDone <-chan time.Time
	type completion struct {
		err      error
		finished time.Time
	}
	var result <-chan completion
	var trace *truststore.ReadTrace
	var stopCheck context.CancelFunc
	defer func() {
		if stopCheck != nil {
			stopCheck()
		}
	}()
	deferred := false
	markDeferred := func() {
		if !deferred {
			record("session_trust_read_deferred")
			deferred = true
		}
	}
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				record("session_certificate_deadline")
			}
			return
		case <-freshness.C:
			if ctx.Err() == nil {
				if trace != nil {
					recordTrace(trace.Snapshot())
				}
				record("session_trust_freshness_expired")
				cancel()
			} else if ctx.Err() == context.DeadlineExceeded {
				record("session_certificate_deadline")
			}
			return
		case <-guardDone:
			if ctx.Err() == nil {
				record("session_guard_exited")
				cancel()
			}
			return
		case <-ticks:
			// Keep the same read after the one-second observation budget. Its
			// result is useful only to this live session before the hard deadline.
			if result != nil {
				continue
			}
			if ctx.Err() != nil {
				continue
			}
			trace = nil
			select {
			case trustReadSlot <- struct{}{}:
			default:
				markDeferred()
				continue
			}
			if recordTrace != nil {
				trace = truststore.NewReadTrace()
			}
			checkCtx, stop := context.WithDeadline(ctx, deadline)
			stopCheck = stop
			ready := make(chan completion, 1)
			result = ready
			go func(readTrace *truststore.ReadTrace) {
				err := func() error {
					defer func() { <-trustReadSlot }()
					return checkMonitoredTrustObserved(checkCtx, func() error { return check(readTrace) }, agent.Wait, readTrace)
				}()
				ready <- completion{err: err, finished: time.Now()}
			}(trace)
			soft.Reset(trustReadTimeout)
			softDone = soft.C
		case <-softDone:
			softDone = nil
			if ctx.Err() != nil {
				continue
			}
			if trace != nil {
				recordTrace(trace.Snapshot())
			}
			markDeferred()
		case completed := <-result:
			result = nil
			softDone = nil
			soft.Stop()
			stopCheck()
			stopCheck = nil
			if ctx.Err() != nil {
				if ctx.Err() == context.DeadlineExceeded {
					record("session_certificate_deadline")
				}
				return
			}
			select {
			case <-guardDone:
				record("session_guard_exited")
				cancel()
				return
			default:
			}
			err := completed.err
			if err == nil {
				if !completed.finished.Before(deadline) || !time.Now().Before(deadline) {
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
				if trace != nil {
					snapshot := trace.Snapshot()
					if snapshot.Elapsed >= 50*time.Millisecond {
						recordTrace(snapshot)
					}
				}
				continue
			}
			if ctx.Err() != nil {
				if ctx.Err() == context.DeadlineExceeded {
					record("session_certificate_deadline")
				}
				return
			}
			if trace != nil {
				recordTrace(trace.Snapshot())
			}
			if errors.Is(err, context.DeadlineExceeded) {
				record("session_trust_freshness_expired")
				cancel()
				return
			}
			if errors.Is(err, errTrustReadTimeout) || errors.Is(err, errTrustReadPending) || err == truststore.ErrStoreBusy {
				markDeferred()
				continue
			}
			code := "session_trust_check_failed"
			switch {
			case errors.Is(err, errMonitorGuardExited):
				code = "session_guard_exited"
			case errors.Is(err, truststore.ErrPeerRevoked):
				code = "session_trust_revoked"
			case errors.Is(err, truststore.ErrPeerNotFound):
				code = "session_trust_peer_missing"
			case errors.Is(err, truststore.ErrStoreNotFound):
				code = "session_trust_store_missing"
			}
			record(code)
			cancel()
			return
		}
	}
}
