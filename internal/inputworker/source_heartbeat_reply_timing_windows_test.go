//go:build windows

package inputworker

import (
	"context"
	"net"
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
			if strings.Contains(event, "_timing_worker_") {
				worker = append(worker, event)
			}
		}
		if foreign && (err != ErrProtocol || len(worker) != 0) || !foreign && (err != nil || len(worker) != 3 || worker[0] != "installed_source_heartbeat_timing_worker_presence_wait_lt1s" || worker[1] != "installed_source_heartbeat_timing_worker_presence_native_lt10ms" || worker[2] != "installed_source_heartbeat_timing_worker_reply_gate_lt1ms") {
			t.Fatal("foreign reply contaminated diagnostic phases or changed outcome", foreign, err, events)
		}
	}
}
