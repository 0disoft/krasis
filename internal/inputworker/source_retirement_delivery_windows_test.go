//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestIdleSourceStopReadsFinalRetirementAfterPipeOwnerDisappears(t *testing.T) {
	for _, retiring := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled-owner", true: "idle-retirement"}[retiring], func(t *testing.T) {
			for _, confirmed := range []bool{false, true} {
				t.Run(map[bool]string{false: "missing-proof", true: "delayed-proof"}[confirmed], func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						server, conn := net.Pipe()
						defer server.Close()
						c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
						defer c.channel.Close()
						c.retiring.Store(retiring)
						c.sequence = 1
						if retiring {
							c.pending = sourcePacket{op: sourceHeartbeat, sequence: 1}
							c.replies <- sourcePacket{op: sourceHeartbeat, reply: 1}
						}
						c.channel.authority.checkInput = func() error { return ErrAuthority }
						if confirmed {
							go func() {
								time.Sleep(35 * time.Millisecond)
								c.retired.Store(true)
								close(c.done)
							}()
						}
						cleanup, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
						defer cancel()
						_, err := c.call(cleanup, sourceStop)
						if (err == nil) != confirmed || c.sequence != 1 || c.epoch.Load() != 0 {
							t.Fatal("retirement lost or absence became proof/new command", err, c.sequence)
						}
						if !confirmed && c.channel.authority.check(false) == nil {
							t.Fatal("unconfirmed cleanup left owner usable")
						}
					})
				})
			}
		})
	}
}
