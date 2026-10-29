package inputworker

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Each process retains at most four native queries, including queries whose
// callers expired. A stuck SCM call cannot create unbounded replacement work.
var authorityQueries = make(chan struct{}, 4)

// Reserve one of those four slots for presence. Full SCM/WTS input checks
// and advisory preflights may own at most three; they cannot consume
// the last slot required by a fresh service/agent heartbeat. This is admission
// only, never a cached proof or permission to exceed the native-query limit.
var authorityProofQueries = make(chan struct{}, 3)

type authorityQuery struct {
	done   chan struct{}
	err    error
	timing *authorityQueryTiming
	phase  atomic.Uint32 // Observation only; never authorizes or cancels work.
}

type authorityQueryKind uint8

const (
	inputAuthorityQuery authorityQueryKind = iota
	presenceAuthorityQuery
	heartbeatAuthorityQuery
	sourceInputAuthorityQuery
	sourcePresenceAuthorityQuery
	sourceAdvisoryAuthorityQuery
	sourceAdvisoryPresenceAuthorityQuery
	startupAuthorityQuery
	sourceWatchdogAuthorityQuery
)

// Only read-only proof queries run here. Input injection stays on its original
// ordered worker. Every command obtains a fresh query; no success is cached.
func (channel *Channel) authorizeInput(ctx context.Context, observed bool) error {
	return channel.queryAuthority(ctx, observed, inputAuthorityQuery)
}

// Service-local startup needs the full fresh proof and its original native
// failure, under the same capacity, cancellation and handle-retirement owner.
func (channel *Channel) authorizeStartup(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, startupAuthorityQuery)
}

// Presence has independent admission so a slow full SCM authorization cannot
// starve actual worker heartbeats. Both lanes retain native handles and share
// the process-wide capacity until their queries really return.
func (channel *Channel) authorizePresence(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, presenceAuthorityQuery)
}

// Unsent optional maintenance never grants input authority. Its expired native
// query keeps its slot/handles without retiring a healthy command owner. New
// commands require independent fresh checks; Close still joins every lane.
func (channel *Channel) authorizeHeartbeatPresence(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, heartbeatAuthorityQuery)
}

func (channel *Channel) authorizeSourceInput(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, sourceInputAuthorityQuery)
}

func (channel *Channel) authorizeSourcePresence(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, sourcePresenceAuthorityQuery)
}

// Background monitoring cannot consume the slot reserved for an actual sent
// heartbeat. It still obtains a fresh pinned-owner proof and retains native
// handles until completion; input authorization never uses this result.
func (channel *Channel) authorizeSourceWatchdogPresence(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, sourceWatchdogAuthorityQuery)
}

func (channel *Channel) authorizeSourceAdvisory(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, sourceAdvisoryAuthorityQuery)
}

// Advisory presence shares the optional query owner, leaving mandatory command
// presence independent. An expired unsent observation never fences input.
func (channel *Channel) authorizeSourceAdvisoryPresence(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, sourceAdvisoryPresenceAuthorityQuery)
}

func (channel *Channel) queryAuthority(ctx context.Context, observed bool, kind authorityQueryKind) error {
	if channel == nil || ctx == nil {
		return ErrAuthority
	}
	var profile *sourcePresenceTiming
	var admitted time.Time
	if kind == sourcePresenceAuthorityQuery {
		profile, _ = ctx.Value(sourcePresenceTimingKey{}).(*sourcePresenceTiming)
		if profile != nil {
			admitted = time.Now()
		}
	}
	// An unsent advisory batch, like an unsent keepalive, grants no input.
	// Keep its timed-out native query owned without fencing unrelated commands.
	// Its caller must discard the batch; later work still needs a fresh check.
	optional := kind == heartbeatAuthorityQuery || kind == sourceAdvisoryAuthorityQuery || kind == sourceAdvisoryPresenceAuthorityQuery
	for {
		if err := ctx.Err(); err != nil {
			return queryPhaseFailure(kind, "admission", err)
		}
		channel.queryMu.Lock()
		if channel.closing || channel.authority == nil || channel.authority.fenced.Load() {
			channel.queryMu.Unlock()
			return queryAdmissionFailure(kind, "closed", nil)
		}
		current := &channel.query
		if kind == sourceWatchdogAuthorityQuery {
			current = &channel.watchdogQuery
		} else if kind == presenceAuthorityQuery || kind == sourcePresenceAuthorityQuery {
			current = &channel.presenceQuery
		} else if optional {
			current = &channel.maintenanceQuery
		}
		if previous := *current; previous != nil {
			channel.queryMu.Unlock()
			select {
			case <-previous.done:
				continue // A different command's success never authorizes this one.
			case <-ctx.Done():
				return queryPhaseFailure(kind, "waiting", ctx.Err())
			}
		}
		if err := channel.reserveQueryCapacityLocked(ctx, kind); err != nil {
			channel.queryMu.Unlock()
			return err
		}
		// A contended reservation releases queryMu. Neither its old lane state
		// nor its old admission decision survives that wait. The reserved slot
		// authorizes no work; the native query below still obtains a fresh proof.
		if channel.closing || channel.authority.fenced.Load() {
			releaseQueryCapacity(kind)
			channel.queryMu.Unlock()
			return queryAdmissionFailure(kind, "closed", nil)
		}
		if err := ctx.Err(); err != nil {
			releaseQueryCapacity(kind)
			channel.queryMu.Unlock()
			return queryCapacityFailure(kind, err)
		}
		if *current != nil {
			releaseQueryCapacity(kind)
			channel.queryMu.Unlock()
			continue
		}
		query := &authorityQuery{done: make(chan struct{})}
		if profile != nil {
			query.timing = &authorityQueryTiming{}
		}
		*current = query
		queryCtx, merge := handoff.ForkAuthority(ctx)
		go func() {
			defer releaseQueryCapacity(kind)
			query.phase.Store(queryChecking)
			if query.timing != nil {
				query.timing.started = time.Now()
			}
			if kind == sourceInputAuthorityQuery || kind == sourceAdvisoryAuthorityQuery {
				query.err = channel.authority.checkSource()
			} else if kind == sourcePresenceAuthorityQuery || kind == sourceAdvisoryPresenceAuthorityQuery || kind == sourceWatchdogAuthorityQuery {
				query.err = channel.authority.checkSourceHeartbeat()
			} else if kind == startupAuthorityQuery {
				query.err = channel.authority.checkResult(nil, false)
			} else if kind != inputAuthorityQuery {
				query.err = channel.authority.checkHeartbeat()
			} else if observed {
				query.err = channel.authority.checkObserved(queryCtx, false)
			} else {
				query.err = channel.authority.check(false)
			}
			if query.timing != nil {
				query.timing.finished = time.Now()
			}
			query.phase.Store(queryPublishing)
			channel.queryMu.Lock()
			if ctx.Err() != nil && !optional {
				channel.closeQueryAdmissionLocked()
				channel.authority.fenced.Store(true)
			}
			*current = nil
			if channel.closing {
				query.err = ErrAuthority
				if !channel.pendingQueriesLocked() {
					// Resource retirement may itself join a native owner. Retain
					// this lifecycle marker until it actually completes, so Close
					// cannot block on closeOnce or claim cleanup prematurely.
					*current = query
					channel.queryMu.Unlock()
					query.phase.Store(queryRetiring)
					_ = channel.closeResources()
					channel.queryMu.Lock()
					*current = nil
				}
			}
			query.phase.Store(queryCompleted)
			close(query.done)
			channel.queryMu.Unlock()
		}()
		channel.queryMu.Unlock()
		select {
		case <-query.done:
			if err := ctx.Err(); err != nil {
				failure := queryPhaseFailure(kind, query.phaseName(), err)
				if !optional {
					channel.cancelQuery(query)
				}
				return failure
			}
			if profile != nil {
				profile.observe(admitted, query.timing)
			}
			merge() // The query can no longer mutate its diagnostic sample.
			return query.err
		case <-ctx.Done():
			failure := queryPhaseFailure(kind, query.phaseName(), ctx.Err())
			if !optional {
				channel.cancelQuery(query)
			}
			return failure
		}
	}
}

// Cancel only the task this caller owns. If its result already completed, there
// is no pending native work to retire. Never start/join cleanup on a caller whose
// deadline has already expired; the retained query owner finishes that work.
func (channel *Channel) cancelQuery(query *authorityQuery) {
	channel.queryMu.Lock()
	defer channel.queryMu.Unlock()
	if channel.query != query && channel.presenceQuery != query && channel.watchdogQuery != query && channel.maintenanceQuery != query {
		return
	}
	channel.closeQueryAdmissionLocked()
	channel.authority.fenced.Store(true)
	if channel.stream != nil {
		_ = channel.stream.SetDeadline(time.Now())
	}
}
