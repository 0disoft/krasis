//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

type failingSourceStream struct {
	net.Conn
	cause    error
	deadline bool
	partial  bool
	writes   int
}

func (s *failingSourceStream) SetWriteDeadline(t time.Time) error {
	if s.deadline {
		return s.cause
	}
	return s.Conn.SetWriteDeadline(t)
}
func (s *failingSourceStream) Write(p []byte) (int, error) {
	s.writes++
	if s.partial {
		return len(p) / 2, s.cause
	}
	return 0, s.cause
}

func TestSourceWriteFailureKeepsCauseWithoutReplayOrAuthority(t *testing.T) {
	for _, tc := range []struct {
		name              string
		cause             error
		deadline, partial bool
		reason            string
	}{
		{"native", syscall.Errno(109), false, false, "native"},
		{"deadline", os.ErrDeadlineExceeded, false, false, "timeout"},
		{"configure", syscall.Errno(6), true, false, "native"},
		{"partial", io.ErrClosedPipe, false, true, "closed"},
		{"short", nil, false, true, "short_write"},
		{"private", errors.New("private-path-input-peer"), false, false, "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &failingSourceStream{Conn: conn, cause: tc.cause, deadline: tc.deadline, partial: tc.partial}
			c := &sourceClient{channel: clientChannel(stream, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
			defer c.channel.Close()
			var events []string
			c.timing = timingSampler{source: true, emit: func(s string) { events = append(events, s) }}
			_, err := c.call(context.Background(), sourceActivate)
			cause := tc.cause
			if cause == nil {
				cause = io.ErrShortWrite
			}
			if !errors.Is(err, ErrAuthority) || !errors.Is(err, cause) || c.epoch.Load() != 0 || c.sequence != 1 || c.channel.authority.check(false) == nil {
				t.Fatal("write failure lost its cause or fencing", err, c.sequence)
			}
			wantWrites := 1
			if tc.deadline {
				wantWrites = 0
			}
			if stream.writes != wantWrites {
				t.Fatal("write was retried", stream.writes)
			}
			found := false
			for _, event := range events {
				if event == "installed_source_io_"+tc.reason {
					found = true
				}
				if strings.Contains(event, "private") {
					t.Fatal("private error leaked", event)
				}
			}
			if !found {
				t.Fatal("write cause classification missing", events)
			}
		})
	}
}
