//go:build windows

package inputworker

import (
	"context"
	"net"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSourceHeartbeatAcceptsTimingOnlyFromMatchingReply(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		server, pipe := net.Pipe()
		c := &sourceClient{channel: clientChannel(pipe, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		var events []string
		c.heartbeatTiming = timingSampler{source: true, heartbeat: true, emit: func(code string) { events = append(events, code) }}
		joined := make(chan struct{})
		go func() {
			defer close(joined)
			request, err := readSourcePacket(server)
			if err == nil {
				reply := sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: request.sequence, timing: [3]byte{4, 2, 1}}
				if foreign {
					reply.reply++
				}
				c.replies <- reply
			}
		}()
		_, err := c.call(context.Background(), sourceHeartbeat)
		c.heartbeatTiming.flush(time.Now())
		<-joined
		_ = server.Close()
		_ = c.channel.Close()
		var worker []string
		for _, event := range events {
			if strings.HasPrefix(event, "installed_source_heartbeat_timing_worker_") {
				worker = append(worker, event)
			}
		}
		if foreign && (err != ErrProtocol || len(worker) != 0) || !foreign && (err != nil || len(worker) != 3 || worker[0] != "installed_source_heartbeat_timing_worker_presence_wait_lt1s" || worker[1] != "installed_source_heartbeat_timing_worker_presence_native_lt10ms" || worker[2] != "installed_source_heartbeat_timing_worker_reply_gate_lt1ms") {
			t.Fatal("foreign reply contaminated diagnostic phases or changed outcome", foreign, err, events)
		}
	}
}

// A failed request must not inherit the successful request's aggregate worker
// timings, including when a foreign reply carries plausible timing buckets.
func TestFailedHeartbeatReportsOnlyItsOwnTiming(t *testing.T) {
	server, pipe := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(pipe, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	var events []string
	c.heartbeatTiming = timingSampler{source: true, heartbeat: true, emit: func(code string) { events = append(events, code) }}
	c.heartbeatTiming.observe([7]byte{1, 1, 1, 1, 2, 3, 4}, time.Now())
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		request, err := readSourcePacket(server)
		if err == nil {
			c.replies <- sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: request.sequence + 1, timing: [3]byte{5, 5, 5}}
		}
	}()
	_, err := c.call(context.Background(), sourceHeartbeat)
	<-joined
	if err != ErrProtocol {
		t.Fatal("foreign heartbeat reply accepted", err)
	}
	for _, stage := range []string{"worker_presence_wait", "worker_presence_native", "worker_reply_gate"} {
		want := "installed_source_heartbeat_failure_timing_" + stage + "_unavailable"
		found := false
		for _, event := range events {
			if event == want {
				found = true
			}
			if strings.HasPrefix(event, "installed_source_heartbeat_failure_timing_"+stage+"_") && event != want {
				t.Fatal("failure inherited unrelated worker timing", events)
			}
		}
		if !found {
			t.Fatal("missing unavailable worker timing", want, events)
		}
	}
	if !strings.Contains(strings.Join(events, " "), "installed_source_heartbeat_timing_worker_presence_native_lt100ms") {
		t.Fatal("prior aggregate was not preserved", events)
	}
}

func TestHeartbeatFailureTimingUsesMeasuredBucketsAndUnavailable(t *testing.T) {
	var events []string
	emitSourceHeartbeatFailureTiming(func(code string) { events = append(events, code) }, [7]byte{1, 2, 3, 4, 5, 0, 255})
	want := []string{
		"installed_source_heartbeat_failure_timing_client_lock_lt1ms",
		"installed_source_heartbeat_failure_timing_client_authority_lt10ms",
		"installed_source_heartbeat_failure_timing_client_write_lt100ms",
		"installed_source_heartbeat_failure_timing_client_response_lt1s",
		"installed_source_heartbeat_failure_timing_worker_presence_wait_ge1s",
		"installed_source_heartbeat_failure_timing_worker_presence_native_unavailable",
		"installed_source_heartbeat_failure_timing_worker_reply_gate_unavailable",
	}
	if !slices.Equal(events, want) {
		t.Fatal("incorrect attempt measurements", events)
	}
}
