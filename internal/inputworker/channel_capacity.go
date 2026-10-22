package inputworker

import (
	"context"
	"errors"
)

// Called and returned with queryMu held. A full process-wide native query
// budget is backpressure, not an authority denial. Wait on the caller's existing
// budget without retaining queryMu: completing queries need it to publish and
// release their slots. No native work or replacement goroutine starts here.
func (channel *Channel) reserveQueryCapacityLocked(ctx context.Context, kind authorityQueryKind) error {
	select {
	case authorityQueries <- struct{}{}:
		return nil
	default:
	}
	if channel.queryClosed == nil {
		channel.queryClosed = make(chan struct{})
	}
	closed := channel.queryClosed
	channel.queryMu.Unlock()
	var err error
	select {
	case authorityQueries <- struct{}{}:
	case <-closed:
		err = queryAdmissionFailure(kind, "closed", nil)
	case <-ctx.Done():
		err = queryCapacityFailure(kind, ctx.Err())
	}
	channel.queryMu.Lock()
	return err
}

// Preserve the existing capacity diagnostic separately from a timeout inside a
// submitted native check. The deadline is also retained for errors.Is, allowing
// an unsent advisory batch to expire without pretending that it was written.
func queryCapacityFailure(kind authorityQueryKind, cause error) error {
	err := queryAdmissionFailure(kind, "capacity", nil)
	if query, ok := err.(*sourceQueryFailure); ok {
		query.cause = errors.Join(query.cause, cause)
		return query
	}
	return errors.Join(err, cause)
}
