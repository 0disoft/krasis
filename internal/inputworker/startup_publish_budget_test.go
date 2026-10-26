package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestServiceReadyCannotPublishAfterOwnerExpiresInsideProof(t *testing.T) {
	for _, limit := range []string{"caller", "lease", "default"} {
		t.Run(limit, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				defer isolateAuthorityCapacity(t)()
				peer, pipe := net.Pipe()
				defer peer.Close()
				stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
				channel := clientChannel(stream, authorityAgent)
				defer channel.Close()
				var closes atomic.Int32
				channel.authority.closeHandles = func() error { closes.Add(1); return nil }
				budget, expires := 50*time.Millisecond, time.Now().Add(time.Minute)
				ctx := context.Background()
				switch limit {
				case "caller":
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, budget)
					defer cancel()
				case "lease":
					expires = time.Now().Add(budget)
				case "default":
					budget = time.Second
				}
				channel.authority.checkInput = func() error { time.Sleep(2 * budget); return nil }
				done := make(chan error, 1)
				go func() { _, err := readStartupReady(peer); done <- err }()
				started := time.Now()
				err := publishStartupReady(ctx, channel, expires, handoff.StartupProfile{})
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != budget {
					t.Errorf("service READY escaped expired ownership: elapsed=%s err=%v", time.Since(started), err)
				}
				_ = pipe.SetReadDeadline(time.Now())
				_ = peer.SetReadDeadline(time.Now())
				if err := <-done; err == nil {
					t.Error("expired owner published a valid READY")
				}
				if !channel.authority.fenced.Load() || channel.Close() != ErrAuthority || closes.Load() != 0 || stream.closes.Load() != 0 {
					t.Error("expired service startup released live native handles or claimed retirement")
				}
				time.Sleep(2 * budget)
				synctest.Wait()
				if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 {
					t.Error("late startup proof failed exact-once handle retirement")
				}
			})
		})
	}
}

func TestServiceReadyWriteUsesRemainingProofBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, pipe := net.Pipe()
		defer peer.Close()
		agent := clientChannel(pipe, authorityAgent)
		defer agent.Close()
		agent.authority.checkInput = func() error { time.Sleep(25 * time.Millisecond); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		err := publishStartupReady(ctx, agent, time.Now().Add(time.Minute), handoff.StartupProfile{})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond {
			t.Fatal("service READY restarted its write budget", time.Since(started), err)
		}
	})
}

func TestServiceReadyCannotBypassProofCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		_, finish := occupyAuthorityQueries(t)
		defer finish()
		peer, pipe := net.Pipe()
		defer peer.Close()
		agent := clientChannel(pipe, authorityAgent)
		defer agent.Close()
		var checks atomic.Int32
		agent.authority.checkInput = func() error { checks.Add(1); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		err := publishStartupReady(ctx, agent, time.Now().Add(time.Minute), handoff.StartupProfile{})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond || checks.Load() != 0 {
			t.Fatal("service READY bypassed bounded native query capacity", checks.Load(), err)
		}
	})
}
