//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestBusySourceHeartbeatCannotExpireBeforeSendingAndFenceTheCaptureOwner(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService)}
	defer c.channel.Close()
	c.epoch.Store(42)
	c.mu.Lock()
	// The command owning the mutex finishes after the legacy heartbeat budget.
	// No real input, desktop, process, service or OS setting is involved.
	ctx, cancel := context.WithTimeout(context.Background(), 1100*time.Millisecond)
	defer cancel()
	unlocked := make(chan struct{})
	go func() { <-ctx.Done(); c.mu.Unlock(); close(unlocked) }()
	attempted, err := c.pollHeartbeat(context.Background())
	cancel()
	<-unlocked
	if attempted || err != nil || c.sequence != 0 || c.epoch.Load() != 42 || c.channel.authority.check(false) != nil {
		t.Fatalf("busy maintenance became capture failure before sending: attempted=%v err=%v sequence=%d", attempted, err, c.sequence)
	}
}

func TestSourceHeartbeatDoesNotSkipAnActuallyAttemptedFailure(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p, err := readSourcePacket(server)
		if err == nil {
			c.replies <- sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: p.sequence + 1}
		}
	}()
	attempted, err := c.pollHeartbeat(context.Background())
	<-done
	if !attempted || err != ErrProtocol || c.sequence != 1 || c.channel.authority.check(false) == nil {
		t.Fatalf("sent heartbeat failure ignored: attempted=%v err=%v", attempted, err)
	}
}

func TestCanceledSourceHeartbeatDoesNotSendOrChangeAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &sourceClient{}
	c.epoch.Store(42)
	attempted, err := c.pollHeartbeat(ctx)
	if attempted || err != context.Canceled || c.sequence != 0 || c.epoch.Load() != 42 {
		t.Fatal("canceled heartbeat changed source authority", attempted, err)
	}
}
