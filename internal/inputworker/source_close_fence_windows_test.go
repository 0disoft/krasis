//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

type sourceCloseFailureConn struct{ net.Conn }

func (conn sourceCloseFailureConn) Close() error {
	_ = conn.Conn.Close()
	return errors.New("close fixture failed")
}

func TestSourceCloseKeepsResourceFailureAheadOfCleanRetirement(t *testing.T) {
	for _, failing := range []string{"pipe", "proof"} {
		t.Run(failing, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			var stream net.Conn = conn
			if failing == "pipe" {
				stream = sourceCloseFailureConn{conn}
			}
			live := &sourceClient{channel: clientChannel(stream, authorityService), done: make(chan struct{})}
			if failing == "proof" {
				live.channel.authority.closeHandles = func() error { return ErrAuthority }
			}
			live.retired.Store(true)
			live.err = wininput.ErrEmergencyRelease
			close(live.done)
			prior := errors.Join(context.Canceled, ErrSourceCaptureRetired)
			err := live.closeResult(prior)
			if !errors.Is(err, ErrRecovery) || !errors.Is(err, context.Canceled) ||
				!errors.Is(err, wininput.ErrEmergencyRelease) || CleanSourceRetirement(err) {
				t.Fatal("resource failure lost behind cancellation/retirement", err)
			}
		})
	}
}

func TestSourceStartupFailureRetainsChannelCleanupFence(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	channel := clientChannel(sourceCloseFailureConn{conn}, authorityService)
	if err := closeSourceChannelResult(channel, context.DeadlineExceeded); !errors.Is(err, ErrRecovery) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("startup failure hid uncertain resource cleanup", err)
	}
}

func TestSourceCloseRechecksPendingQueryAfterReaderJoin(t *testing.T) {
	for _, finishes := range []bool{false, true} {
		t.Run(map[bool]string{false: "still-pending", true: "joined-during-close"}[finishes], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, pipe := net.Pipe()
				defer server.Close()
				channel := clientChannel(pipe, authorityService)
				var closes atomic.Int32
				channel.authority.closeHandles = func() error { closes.Add(1); return nil }
				release := make(chan struct{})
				channel.authority.checkPresent = func() error { <-release; return nil }
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				defer cancel()
				if err := channel.authorizeHeartbeatPresence(ctx); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("pending query fixture did not time out", err)
				}
				live := &sourceClient{channel: channel, done: make(chan struct{})}
				live.retired.Store(true)
				finished := make(chan error, 1)
				go func() { finished <- live.closeResult(context.Canceled) }()
				synctest.Wait()
				if closes.Load() != 0 {
					t.Error("close released handles still owned by native query")
				}
				if finishes {
					close(release)
					synctest.Wait()
				}
				close(live.done)
				err := <-finished
				if !errors.Is(err, context.Canceled) || errors.Is(err, ErrRecovery) == finishes {
					t.Error("source cleanup used stale or missing retirement proof", err)
				}
				if !finishes {
					close(release)
					synctest.Wait()
				}
				if channel.Close() != nil || closes.Load() != 1 || !channel.authority.fenced.Load() {
					t.Fatal("late query leaked handles or revived authority")
				}
			})
		})
	}
}
