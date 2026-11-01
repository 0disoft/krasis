package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestClientCloseRechecksRetirementAfterInputJoin(t *testing.T) {
	for _, state := range []string{"retired", "pending", "close-failed"} {
		t.Run(state, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				defer isolateAuthorityCapacity(t)()
				server, pipe := net.Pipe()
				defer server.Close()
				channel := clientChannel(pipe, authorityService)
				client := &Client{channel: channel}
				entered, release := make(chan struct{}), make(chan struct{})
				var handlesClosed atomic.Int32
				channel.authority.checkInput = func() error { close(entered); <-release; return nil }
				channel.authority.closeHandles = func() error {
					handlesClosed.Add(1)
					if state == "close-failed" {
						return ErrAuthority
					}
					return nil
				}
				queryDone := make(chan error, 1)
				go func() { queryDone <- channel.authorizeInput(context.Background(), false) }()
				<-entered
				// Close first interrupts the channel, then joins this input owner.
				client.mu.Lock()
				closed := make(chan error, 1)
				go func() { closed <- client.Close() }()
				synctest.Wait()
				if !channel.authority.fenced.Load() || handlesClosed.Load() != 0 {
					t.Error("pending native proof was not fenced and pinned")
				}
				if state != "pending" {
					close(release)
					<-queryDone
					synctest.Wait()
				}
				client.mu.Unlock()
				err := <-closed
				if (err == nil) != (state == "retired") || err != nil && !errors.Is(err, ErrAuthority) {
					t.Errorf("wrong retirement verdict: state=%s err=%v", state, err)
				}
				if !client.failed {
					t.Error("closed client could accept replacement input")
				}
				if state == "pending" {
					if handlesClosed.Load() != 0 {
						t.Error("unreturned native query lost pinned handles")
					}
					close(release)
					<-queryDone
					synctest.Wait()
				}
				if handlesClosed.Load() != 1 {
					t.Error("native handles were not retired exactly once")
				}
			})
		})
	}
}
