package inputworker

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Each process retains at most four native queries, including queries whose
// callers expired. A stuck SCM call cannot create unbounded replacement work.
var authorityQueries = make(chan struct{}, 4)

type authorityQuery struct {
	done chan struct{}
	err  error
}

type authorityQueryKind uint8

const (
	inputAuthorityQuery authorityQueryKind = iota
	presenceAuthorityQuery
	heartbeatAuthorityQuery
	sourceInputAuthorityQuery
	sourcePresenceAuthorityQuery
	sourceAdvisoryAuthorityQuery
)

// Only read-only proof queries run here. Input injection stays on its original
// ordered worker. Every command obtains a fresh query; no success is cached.
func (channel *Channel) authorizeInput(ctx context.Context, observed bool) error {
	return channel.queryAuthority(ctx, observed, inputAuthorityQuery)
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

func (channel *Channel) authorizeSourceAdvisory(ctx context.Context) error {
	return channel.queryAuthority(ctx, false, sourceAdvisoryAuthorityQuery)
}

func (channel *Channel) queryAuthority(ctx context.Context, observed bool, kind authorityQueryKind) error {
	if channel == nil || ctx == nil {
		return ErrAuthority
	}
	// An unsent advisory batch, like an unsent keepalive, grants no input.
	// Keep its timed-out native query owned without fencing unrelated commands.
	// Its caller must discard the batch; later work still needs a fresh check.
	optional := kind == heartbeatAuthorityQuery || kind == sourceAdvisoryAuthorityQuery
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(ErrAuthority, err)
		}
		channel.queryMu.Lock()
		if channel.closing || channel.authority == nil || channel.authority.fenced.Load() {
			channel.queryMu.Unlock()
			return ErrAuthority
		}
		current := &channel.query
		if kind == presenceAuthorityQuery || kind == sourcePresenceAuthorityQuery {
			current = &channel.presenceQuery
		} else if kind == heartbeatAuthorityQuery || kind == sourceAdvisoryAuthorityQuery {
			current = &channel.maintenanceQuery
		}
		if previous := *current; previous != nil {
			channel.queryMu.Unlock()
			select {
			case <-previous.done:
				continue // A different command's success never authorizes this one.
			case <-ctx.Done():
				return errors.Join(ErrAuthority, ctx.Err())
			}
		}
		select {
		case authorityQueries <- struct{}{}:
		default:
			channel.queryMu.Unlock()
			return ErrAuthority
		}
		query := &authorityQuery{done: make(chan struct{})}
		*current = query
		queryCtx, merge := handoff.ForkAuthority(ctx)
		go func() {
			defer func() { <-authorityQueries }()
			if kind == sourceInputAuthorityQuery || kind == sourceAdvisoryAuthorityQuery {
				query.err = channel.authority.checkSource()
			} else if kind == sourcePresenceAuthorityQuery {
				query.err = channel.authority.checkSourceHeartbeat()
			} else if kind != inputAuthorityQuery {
				query.err = channel.authority.checkHeartbeat()
			} else if observed {
				query.err = channel.authority.checkObserved(queryCtx, false)
			} else {
				query.err = channel.authority.check(false)
			}
			channel.queryMu.Lock()
			if ctx.Err() != nil && !optional {
				channel.closing = true
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
					_ = channel.closeResources()
					channel.queryMu.Lock()
					*current = nil
				}
			}
			close(query.done)
			channel.queryMu.Unlock()
		}()
		channel.queryMu.Unlock()
		select {
		case <-query.done:
			if err := ctx.Err(); err != nil {
				if !optional {
					channel.cancelQuery(query)
				}
				return errors.Join(ErrAuthority, err)
			}
			merge() // The query can no longer mutate its diagnostic sample.
			return query.err
		case <-ctx.Done():
			if !optional {
				channel.cancelQuery(query)
			}
			return errors.Join(ErrAuthority, ctx.Err())
		}
	}
}

// Cancel only the task this caller owns. If its result already completed, there
// is no pending native work to retire. Never start/join cleanup on a caller whose
// deadline has already expired; the retained query owner finishes that work.
func (channel *Channel) cancelQuery(query *authorityQuery) {
	channel.queryMu.Lock()
	defer channel.queryMu.Unlock()
	if channel.query != query && channel.presenceQuery != query && channel.maintenanceQuery != query {
		return
	}
	channel.closing = true
	channel.authority.fenced.Store(true)
	if channel.stream != nil {
		_ = channel.stream.SetDeadline(time.Now())
	}
}
