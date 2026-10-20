package inputworker

import (
	"context"
	"time"
)

// Both native children have independently authenticated their fixed installed
// capabilities before this entry. The journal never binds a native desktop or
// queries SCM/WTS on live input/recovery. Readiness remains the native transports'
// successful startup, not the mere existence of this parent.
func serveIsolatedJournal(ctx context.Context, channel *Channel, input *isolatedInput, lease Lease, ready func() error) (Report, error) {
	if channel == nil || channel.authority == nil || channel.authority.kind != authorityAgent || input == nil || input.recoveryRetire == nil || !validLease(ctx, channel.stream, input, lease, time.Now()) {
		return Report{}, ErrAuthority
	}
	defer channel.Close()
	if ready != nil {
		if err := ready(); err != nil {
			return Report{}, err
		}
	}
	timing := &workerTiming{}
	return serve(ctx, channel.stream, input, lease, journalExecution(input, nil, timing), nil, time.Now, timing)
}
