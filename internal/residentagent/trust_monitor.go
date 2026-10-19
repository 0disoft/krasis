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
	"prelock_lockfile_info", "prelock_lockfile_owner", "prelock_lockfile_acl_query", "prelock_lockfile_acl_check",
	"lock", "store_read", "store_read_open", "store_read_acl", "store_read_bytes",
	"store_read_decode", "store_read_close", "certificate", "certificate_open", "certificate_acl", "certificate_bytes", "certificate_close", "certificate_validate", "certificate_match", "retry_wait",
}

// Emit fixed, coarse timing codes so exported diagnostics locate a slow phase
// without exposing peer identity, paths, certificates, or raw error text.
func recordTrustTrace(snapshot truststore.ReadTraceSnapshot, record func(string)) {
	for _, phase := range trustReadPhases {
		duration, observed := snapshot.Durations[phase]
		if !observed {
			continue
		}
		if (strings.HasPrefix(phase, "prelock_") || strings.HasPrefix(phase, "store_read_") || strings.HasPrefix(phase, "certificate_")) &&
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
