//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// Model cancellation racing the final deadline reset after both fresh proofs.
// Close retains a native owner until release, making callback lifetime visible.
type startupCancelHandoffConn struct {
	net.Conn
	cancel           context.CancelFunc
	closing, release chan struct{}
	once             sync.Once
}

func (c *startupCancelHandoffConn) SetReadDeadline(deadline time.Time) error {
	if deadline.IsZero() {
		c.cancel()
		<-c.closing
		return nil
	}
	return c.Conn.SetReadDeadline(deadline)
}

func (c *startupCancelHandoffConn) Close() error {
	c.once.Do(func() { close(c.closing); <-c.release })
	return c.Conn.Close()
}

func TestAgentRequestCannotPublishSuccessDuringCancellationClose(t *testing.T) {
	for _, preparation := range []bool{false, true} {
		name := "lease"
		if preparation {
			name = "preparation"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				peer, pipe := net.Pipe()
				defer peer.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				stream := &startupCancelHandoffConn{Conn: pipe, cancel: cancel, closing: make(chan struct{}), release: make(chan struct{})}
				owner := clientChannel(stream, authorityAgent)
				defer owner.Close()
				var handlesClosed atomic.Int32
				owner.authority.closeHandles = func() error { handlesClosed.Add(1); return nil }
				type result struct {
					prepared bool
					err      error
				}
				returned := make(chan result, 1)
				go func() { _, prepared, err := receiveAgentRequest(ctx, owner); returned <- result{prepared, err} }()
				packet := encodePreparation()
				if !preparation {
					var err error
					packet, err = encodeLease(testLease(), time.Now())
					if err != nil {
						t.Fatal(err)
					}
				}
				if _, err := peer.Write(packet[:]); err != nil {
					t.Fatal(err)
				}
				<-stream.closing
				synctest.Wait()
				select {
				case got := <-returned:
					t.Error("startup returned while its cancellation callback still owned the channel", got)
					returned <- got
				default:
				}
				if handlesClosed.Load() != 0 {
					t.Error("test lost the pending native cleanup owner")
				}
				close(stream.release)
				got := <-returned
				if got.prepared || !errors.Is(got.err, ErrAuthority) {
					t.Error("canceled startup published readiness", got)
				}
				if handlesClosed.Load() != 1 || owner.Close() != nil {
					t.Error("startup result escaped before exact channel retirement")
				}
			})
		})
	}
}
