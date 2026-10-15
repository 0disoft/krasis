//go:build windows

package inputworker

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSourceFailedReplyKeepsFailureAndFlushesBoundedTiming(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	var events []string
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	c.timing = timingSampler{source: true, emit: func(code string) { events = append(events, code) }}
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		p, err := readSourcePacket(server)
		if err == nil {
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence + 1}
		}
	}()
	_, err := c.call(context.Background(), sourceActivate)
	<-joined
	if err != ErrProtocol || c.epoch.Load() != 0 || c.sequence != 1 {
		t.Fatal("timing changed fencing, reply identity or replay policy", err)
	}
	if len(events) != 5 || events[4] != "installed_source_failure_protocol" {
		t.Fatal("failure lost phase or partial timings", events)
	}
	for _, code := range events[:4] {
		if !strings.HasPrefix(code, "installed_source_timing_client_") {
			t.Fatal("source timing contaminated injection profile", code)
		}
	}
}

func TestSourceHeartbeatTimingCannotContaminateCommands(t *testing.T) {
	var events []string
	emit := func(code string) { events = append(events, code) }
	command := timingSampler{source: true, emit: emit}
	heartbeat := timingSampler{source: true, heartbeat: true, emit: emit}
	now := time.Now()
	for i := 0; i < 100; i++ {
		command.observe([7]byte{1, 2, 1, 4}, now)
		heartbeat.observe([7]byte{0, 1, 1, 2}, now)
	}
	if len(events) != 0 {
		t.Fatal("per-command logging escaped aggregate window")
	}
	command.flush(now)
	heartbeat.flush(now)
	if len(events) != 7 || events[3] != "installed_source_timing_client_response_lt1s" || events[6] != "installed_source_heartbeat_timing_client_response_lt10ms" {
		t.Fatal("source/heartbeat maximum buckets lost or mixed", events)
	}
}
