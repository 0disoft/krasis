//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
)

func TestHeartbeatProgressPreservesAuthorityFailureAndSeparatesWatchdog(t *testing.T) {
	for _, failService := range []bool{false, true} {
		left, right := net.Pipe()
		service, agent := clientChannel(left, authorityService), clientChannel(right, authorityAgent)
		var progress sourceHeartbeatProgress
		service.authority.checkPresent = func() error {
			if failService {
				return ErrAuthority
			}
			return nil
		}
		agent.authority.checkPresent = func() error { return ErrAuthority }
		err := authorizeSourceReadProgress(context.Background(), service, agent, sourceHeartbeat, &progress)
		want := heartbeatAgentPresence
		if failService {
			want = heartbeatServicePresence
		}
		if err == nil || progress.stage != want || progress.slowBucket == 0 {
			t.Fatal("authority failure lost its stage", err, progress)
		}
		before := progress
		_ = authorizeSourceRead(context.Background(), service, agent, sourceHeartbeat)
		if progress != before {
			t.Fatal("watchdog overwrote command progress")
		}
		_ = service.Close()
		_ = agent.Close()
	}
}

func TestHeartbeatWriteFailureProgressSurvivesRetirement(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded}
	var progress sourceHeartbeatProgress
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}, heartbeat: &progress}
	defer w.channel.Close()
	request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 1, binding: w.binding}
	if err := w.sendCompletedReply(request, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}); !errors.Is(err, os.ErrDeadlineExceeded) || progress.stage != heartbeatReplyWrite {
		t.Fatal("failed heartbeat misclassified", err, progress)
	}
	before := progress
	_ = w.write(sourcePacket{kind: sourceRetired}, nil, true)
	if progress != before || !w.broken || stream.writes != 1 {
		t.Fatal("retirement overwrote failure or replayed write", progress, stream.writes)
	}
}
