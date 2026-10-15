//go:build windows

package inputworker

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSlowUnsentHeartbeatCannotBlockAuthenticatedActivation(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var checks atomic.Int32
	c.channel.authority.checkInput = func() error {
		if checks.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	}
	heartbeat := make(chan error, 1)
	go func() {
		attempted, err := c.pollHeartbeat(context.Background())
		if attempted && err == nil {
			err = ErrProtocol
		}
		heartbeat <- err
	}()
	<-entered
	wire := make(chan sourcePacket, 1)
	go func() {
		p, err := readSourcePacket(server)
		if err == nil {
			wire <- p
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
		}
	}()
	activation := make(chan error, 1)
	go func() { _, err := c.call(context.Background(), sourceActivate); activation <- err }()
	select {
	case err := <-activation:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("activation waited for unrelated heartbeat authentication")
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-heartbeat; err != nil {
		t.Fatal(err)
	}
	if p := <-wire; p.op != sourceActivate || p.sequence != 1 || c.sequence != 1 || c.epoch.Load() != 1 {
		t.Fatal("unsent maintenance consumed sequence or capture authority", p)
	}
}

func TestExpiredHeartbeatPreflightSendsNothingAndPreservesOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, done: make(chan struct{})}
		defer c.channel.Close()
		c.channel.authority.checkInput = func() error { time.Sleep(1100 * time.Millisecond); return nil }
		if attempted, err := c.pollHeartbeat(context.Background()); attempted || err != nil {
			t.Fatal("expired unsent heartbeat became a transport failure", attempted, err)
		}
		if c.sequence != 0 || !c.lastReplyRequestAt.IsZero() || c.channel.authority.closed {
			t.Fatal("unsent preflight changed liveness, sequence or proof lifetime")
		}
	})
}

func TestHeartbeatCheckCannotAuthorizeCaptureOrBeReused(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService)}
	defer c.channel.Close()
	proof := &sourceHeartbeatCheck{owner: c}
	if proof.validate(c, context.Background(), sourceActivate) == nil {
		t.Fatal("heartbeat proof authorized capture")
	}
	if proof.validate(c, context.Background(), sourceHeartbeat) != nil {
		t.Fatal("fresh heartbeat proof rejected")
	}
	if proof.validate(c, context.Background(), sourceHeartbeat) == nil {
		t.Fatal("heartbeat proof reused")
	}
	proof = &sourceHeartbeatCheck{owner: c}
	_ = c.channel.authority.Close()
	if proof.validate(c, context.Background(), sourceHeartbeat) == nil {
		t.Fatal("closed authority accepted preflight")
	}
}

func TestSourceCommandExpiresDuringAuthenticationWithoutStartingRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, done: make(chan struct{})}
		defer c.channel.Close()
		c.channel.authority.checkInput = func() error { time.Sleep(200 * time.Millisecond); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if _, err := c.call(ctx, sourceActivate); err != context.DeadlineExceeded {
			t.Fatal("expired authentication did not retain the caller outcome", err)
		}
		if c.sequence != 0 || c.epoch.Load() != 0 || c.channel.authority.closed {
			t.Fatal("unsent activation consumed an epoch or retired the owner")
		}
	})
}
