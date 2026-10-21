//go:build windows

package inputworker

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceCommandAuthorizationDoesNotStarveHeartbeat(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	c.channel.authority.checkInput = func() error { close(entered); <-release; return nil }
	c.channel.authority.checkPresent = func() error { return nil }
	var events []string
	c.timing.emit = func(code string) { events = append(events, code) }
	command := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _, err := c.call(ctx, sourceActivate); command <- err }()
	<-entered
	c.channel.queryMu.Lock()
	query := c.channel.query
	c.channel.queryMu.Unlock()
	wire := make(chan sourcePacket, 1)
	go func() {
		p, err := readSourcePacket(server)
		if err == nil {
			wire <- p
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
		}
	}()
	attempted, err := c.pollHeartbeat(context.Background())
	// Cancel the unsent command before releasing its native query. It must not
	// consume a sequence or invalidate the heartbeat already acknowledged.
	// The pending query must retain its handles until native return.
	cancel()
	once.Do(func() { close(release) })
	commandErr := <-command
	<-query.done
	if !attempted || err != nil || commandErr != context.Canceled {
		t.Fatal("slow unsent command starved heartbeat", attempted, err, commandErr)
	}
	if len(events) != 1 || events[0] != "installed_source_command_preflight_expired" {
		t.Fatal("unsent native query expiry was not recorded", events)
	}
	select {
	case p := <-wire:
		if p.op != sourceHeartbeat || p.sequence != 1 || c.sequence != 1 || c.epoch.Load() != 0 || !c.channel.authority.closed {
			t.Fatal("unsent command altered heartbeat or capture ownership", p)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat was not written")
	}
}

func TestSourceCommandReauthorizesAfterOutstandingHeartbeat(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	c.channel.authority.checkPresent = func() error { return nil }
	var checks atomic.Int32
	c.channel.authority.checkInput = func() error {
		if checks.Add(1) > 1 {
			return ErrAuthority
		}
		return nil
	}
	sent, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	go func() {
		p, err := readSourcePacket(server)
		if err == nil {
			close(sent)
			<-release
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
		}
	}()
	heartbeat := make(chan error, 1)
	go func() { _, err := c.pollHeartbeat(context.Background()); heartbeat <- err }()
	<-sent
	command := make(chan error, 1)
	go func() { _, err := c.call(context.Background(), sourceActivate); command <- err }()
	deadline := time.Now().Add(time.Second)
	for c.commandWaiting.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.commandWaiting.Load() == 0 {
		t.Fatal("command did not reach the outstanding request gate")
	}
	once.Do(func() { close(release) })
	if err := <-heartbeat; err != nil {
		t.Fatal(err)
	}
	if err := <-command; err != ErrAuthority || checks.Load() != 2 || c.sequence != 1 || c.epoch.Load() != 0 {
		t.Fatal("full authority survived contention or activation was written", err, checks.Load(), c.sequence)
	}
}

func TestSourceCommandProofBindsOwnerOperationAndSingleUse(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService)}
	defer c.channel.Close()
	proof := &sourceCommandCheck{owner: c, op: sourceAdmit}
	if proof.validate(&sourceClient{}, context.Background(), sourceAdmit) == nil || proof.validate(c, context.Background(), sourceActivate) == nil {
		t.Fatal("command proof escaped its exact owner/operation")
	}
	if proof.validate(c, context.Background(), sourceAdmit) != nil || proof.validate(c, context.Background(), sourceAdmit) == nil {
		t.Fatal("command proof was rejected or reused")
	}
	proof = &sourceCommandCheck{owner: c, op: sourceAdmit}
	c.channel.authority.checkPresent = func() error { return ErrAuthority }
	if proof.validate(c, context.Background(), sourceAdmit) == nil {
		t.Fatal("exited pinned service accepted a command")
	}
}
