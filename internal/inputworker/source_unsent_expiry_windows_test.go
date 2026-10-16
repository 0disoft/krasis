//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
)

// Expire between the entry check and proof admission, without a timing race.
type sourceAdmissionExpiry struct {
	context.Context
	checks int
}

func (c *sourceAdmissionExpiry) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.DeadlineExceeded
	}
	return nil
}

func TestSourceUnsentProofExpiryPreservesOwner(t *testing.T) {
	for _, heartbeat := range []bool{false, true} {
		t.Run(map[bool]string{false: "command", true: "heartbeat"}[heartbeat], func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			c := &sourceClient{channel: clientChannel(conn, authorityService), done: make(chan struct{})}
			defer c.channel.Close()
			c.epoch.Store(42)
			ctx := &sourceAdmissionExpiry{Context: context.Background()}
			var err error
			if heartbeat {
				_, err = c.callLockedChecked(ctx, sourceHeartbeat, &sourceHeartbeatCheck{owner: c})
			} else {
				_, err = c.callLockedAuthorized(ctx, sourceActivate, nil, &sourceCommandCheck{owner: c, op: sourceActivate})
			}
			if err != context.DeadlineExceeded || c.sequence != 0 || c.epoch.Load() != 42 || c.channel.authority.closed {
				t.Fatalf("unsent expiry retired authenticated owner: err=%v sequence=%d epoch=%d closed=%v", err, c.sequence, c.epoch.Load(), c.channel.authority.closed)
			}
		})
	}
}
