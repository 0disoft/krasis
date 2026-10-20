//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceActivationAcceptsDelayedExactReplyWithinOwnerBudget(t *testing.T) {
	for _, owner := range []string{"standby", "active", "cursor"} {
		t.Run(owner, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, conn := net.Pipe()
				defer server.Close()
				c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
				defer c.channel.Close()
				c.lastReplyRequestAt = time.Now().Add(-500 * time.Millisecond)
				if owner == "active" {
					c.epoch.Store(42)
				}
				c.cursorPrepared = owner == "cursor"
				go func() {
					p, err := readSourcePacket(server)
					if err == nil {
						time.Sleep(1200 * time.Millisecond)
						c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
					}
				}()
				started := time.Now()
				source := &CaptureSource{current: c}
				if err := source.ActivateProfile(context.Background()); err != nil {
					t.Fatal("valid activation forced recovery", err)
				}
				if c.epoch.Load() != 1 || c.sequence != 1 || c.channel.authority.closed || !c.lastReplyRequestAt.Equal(started) {
					t.Fatal("activation lost its exact reply or renewed from reply completion")
				}
			})
		})
	}
}

func TestSourceActivationMissingReplyKeepsOwnerAndCallerDeadlines(t *testing.T) {
	for _, owner := range []string{"standby", "active", "cursor", "caller", "expired"} {
		t.Run(owner, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, conn := net.Pipe()
				defer server.Close()
				c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
				defer c.channel.Close()
				c.lastReplyRequestAt = time.Now().Add(-1500 * time.Millisecond)
				previous := c.lastReplyRequestAt
				wait := IdleTimeout
				if owner == "active" || owner == "expired" {
					c.epoch.Store(42)
				}
				c.cursorPrepared = owner == "cursor"
				if owner == "active" || owner == "cursor" {
					wait = 500 * time.Millisecond
				}
				ctx := context.Background()
				if owner == "caller" {
					var cancel context.CancelFunc
					wait = 250 * time.Millisecond
					ctx, cancel = context.WithTimeout(ctx, wait)
					defer cancel()
				}
				if owner == "expired" {
					c.lastReplyRequestAt = time.Now().Add(-IdleTimeout)
					previous, wait = c.lastReplyRequestAt, 0
				} else {
					go func() { _, _ = readSourcePacket(server) }()
				}
				started := time.Now()
				source := &CaptureSource{current: c}
				if err := source.ActivateProfile(ctx); err == nil || time.Since(started) != wait {
					t.Fatal("activation escaped its deadline", err, time.Since(started), wait)
				}
				if c.epoch.Load() != 0 || !c.channel.authority.closed || !c.lastReplyRequestAt.Equal(previous) || c.retiring.Load() {
					t.Fatal("missing activation reply preserved authority or renewed liveness")
				}
				if owner == "expired" && c.sequence != 0 {
					t.Fatal("expired owner sent activation")
				}
			})
		})
	}
}
