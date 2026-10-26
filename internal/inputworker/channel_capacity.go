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
	if queryUsesProofCapacity(kind) {
		if err := channel.reserveQueryPoolLocked(ctx, kind, authorityProofQueries); err != nil {
			return err
		}
	}
	err := channel.reserveQueryPoolLocked(ctx, kind, authorityQueries)
	if err != nil && queryUsesProofCapacity(kind) {
		<-authorityProofQueries
	}
	return err
}

// Acquire the restricted pool first so waiting full proofs cannot hoard the
// shared slots. Both waits observe the same caller deadline and channel close.
func (channel *Channel) reserveQueryPoolLocked(ctx context.Context, kind authorityQueryKind, pool chan struct{}) error {
	select {
	case pool <- struct{}{}:
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
	case pool <- struct{}{}:
	case <-closed:
		err = queryAdmissionFailure(kind, "closed", nil)
	case <-ctx.Done():
		err = queryCapacityFailure(kind, ctx.Err())
	}
	channel.queryMu.Lock()
	return err
}

func queryUsesProofCapacity(kind authorityQueryKind) bool {
	return kind != presenceAuthorityQuery && kind != sourcePresenceAuthorityQuery && kind != heartbeatAuthorityQuery
}

// Release both reservations only after the native owner and any resource
// retirement have returned, including when the original caller timed out.
func releaseQueryCapacity(kind authorityQueryKind) {
	<-authorityQueries
	if queryUsesProofCapacity(kind) {
		<-authorityProofQueries
	}
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
