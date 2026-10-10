package residentagent

import (
	"context"
	"time"
)

// A renewable product key has no credential expiry on the connection owner.
// Wire leaves are checked on every handshake; revocation is still monitored.
// Input leases retain their separate bounded duration and cleanup rules.
func productConnectionContext(parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if deadline.IsZero() {
		return context.WithCancel(parent)
	}
	return context.WithDeadline(parent, deadline)
}
