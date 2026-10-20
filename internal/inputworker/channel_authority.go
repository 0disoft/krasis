package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/handoff"
)

// Each process retains at most four native queries, including queries whose
// callers expired. A stuck SCM call cannot create unbounded replacement work.
var authorityQueries = make(chan struct{}, 4)

type authorityQuery struct {
	done chan struct{}
	err  error
}

// Only read-only proof queries run here. Input injection stays on its original
// ordered worker. Every command obtains a fresh query; no success is cached.
func (channel *Channel) authorizeInput(ctx context.Context, observed bool) error {
	if channel == nil || ctx == nil {
		return ErrAuthority
	}
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(ErrAuthority, err)
		}
		channel.queryMu.Lock()
		if channel.closing || channel.authority == nil || channel.authority.fenced.Load() {
			channel.queryMu.Unlock()
			return ErrAuthority
		}
		if previous := channel.query; previous != nil {
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
		channel.query = query
		queryCtx, merge := handoff.ForkAuthority(ctx)
		go func() {
			defer func() { <-authorityQueries }()
			if observed {
				query.err = channel.authority.checkObserved(queryCtx, false)
			} else {
				query.err = channel.authority.check(false)
			}
			channel.queryMu.Lock()
			if ctx.Err() != nil {
				channel.closing = true
				channel.authority.fenced.Store(true)
			}
			if channel.closing {
				channel.queryMu.Unlock()
				_ = channel.closeResources()
				channel.queryMu.Lock()
				query.err = ErrAuthority
			}
			channel.query = nil
			close(query.done)
			channel.queryMu.Unlock()
		}()
		channel.queryMu.Unlock()
		select {
		case <-query.done:
			if err := ctx.Err(); err != nil {
				_ = channel.Close()
				return errors.Join(ErrAuthority, err)
			}
			merge() // The query can no longer mutate its diagnostic sample.
			return query.err
		case <-ctx.Done():
			_ = channel.Close()
			return errors.Join(ErrAuthority, ctx.Err())
		}
	}
}
