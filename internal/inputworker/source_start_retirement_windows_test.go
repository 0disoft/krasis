//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestSourceStartRetainsCleanRetirementBeforeOrDuringHandshake(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before command", false: "during reply"}[before], func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			finished := make(chan struct{})
			c := &sourceClient{channel: clientChannel(sourceWriteBarrier{Conn: conn, ready: finished}, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
			defer c.channel.Close()
			if before {
				c.retired.Store(true)
				close(c.done)
				close(finished)
			} else {
				go func() {
					p, err := readSourcePacket(server)
					if err != nil || p.op != sourceStart {
						t.Error("unexpected startup command", p.op, err)
					}
					// The authenticated reader has received the terminal frame;
					// no start ACK exists and no native input was requested.
					c.retired.Store(true)
					close(c.done)
					close(finished)
				}()
			}
			_, err := c.call(context.Background(), sourceStart)
			<-finished
			want := uint64(1)
			if before {
				want = 0
			}
			if !CleanSourceRetirement(err) || c.sequence != want || c.epoch.Load() != 0 {
				t.Fatal("clean retirement became startup failure or replayed input", err, c.sequence)
			}
		})
	}
}

func TestSourceStartRetirementCannotOverrideMissingProofOrObservedFailure(t *testing.T) {
	for _, mode := range []string{"missing", "cleanup", "emergency", "rejected", "foreign", "activate"} {
		t.Run(mode, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			finished := make(chan struct{})
			c := &sourceClient{channel: clientChannel(sourceWriteBarrier{Conn: conn, ready: finished}, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
			defer c.channel.Close()
			go func() {
				p, err := readSourcePacket(server)
				if err != nil {
					t.Error(err)
				}
				if mode == "cleanup" {
					c.err = ErrRecovery
				}
				if mode == "emergency" {
					c.err = errors.New("emergency stop")
				}
				if mode == "rejected" || mode == "foreign" {
					reply := sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence, status: 1}
					if mode == "foreign" {
						reply.reply++
					}
					c.replies <- reply
				}
				c.retired.Store(mode != "missing")
				close(c.done)
				close(finished)
			}()
			op := sourceStart
			if mode == "activate" {
				op = sourceActivate
			}
			_, err := c.call(context.Background(), op)
			<-finished
			if err == nil || CleanSourceRetirement(err) || c.sequence != 1 || c.epoch.Load() != 0 {
				t.Fatal("unconfirmed handshake/input became clean retirement", mode, err)
			}
		})
	}
}
