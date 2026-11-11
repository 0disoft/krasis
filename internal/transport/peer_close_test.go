package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type countedPeerClose struct {
	net.Conn
	calls atomic.Int32
	err   error
}

func (c *countedPeerClose) Close() error {
	if c.calls.Add(1) != 1 {
		return net.ErrClosed
	}
	_ = c.Conn.Close()
	return c.err
}

func TestPeerCloseIsOneOwnedOperationAndPreservesFailure(t *testing.T) {
	for _, failure := range []error{nil, errors.New("native socket close failed")} {
		left, right := net.Pipe()
		t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
		wire := &countedPeerClose{Conn: left, err: failure}
		peer := &PeerConnection{connection: tls.Client(wire, &tls.Config{})}
		var workers sync.WaitGroup
		for range 8 {
			workers.Go(func() {
				if err := peer.Close(); err != failure {
					t.Errorf("close outcome changed: %v, want %v", err, failure)
				}
			})
		}
		workers.Wait()
		if wire.calls.Load() != 1 {
			t.Fatalf("socket closed %d times", wire.calls.Load())
		}
	}
}

func TestPeerCloseInterruptsIOWithoutWaitingForRemoteRead(t *testing.T) {
	for _, state := range []string{"idle", "reading", "writing", "disconnect_sent"} {
		t.Run(state, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fixture := newTLSFixture(t)
				left, right := net.Pipe()
				t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
				serverTLS := tls.Server(left, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots))
				clientTLS := tls.Client(right, mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
				server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
				if serverErr != nil || clientErr != nil {
					t.Fatalf("authentication: %v / %v", serverErr, clientErr)
				}
				if state == "disconnect_sent" {
					received := make(chan error, 1)
					go func() {
						frame, err := server.ReadFrame()
						if err == nil && (frame.Kind != MessageDisconnect || len(frame.Payload) != 0) {
							t.Error("user-stop frame changed")
						}
						received <- err
					}()
					if err := client.WriteFrameContext(context.Background(), Frame{Kind: MessageDisconnect}); err != nil {
						t.Fatal("disconnect write failed", err)
					}
					if err := <-received; err != nil {
						t.Fatal("disconnect receipt failed", err)
					}
				}
				var pending chan error
				if state == "reading" || state == "writing" {
					pending = make(chan error, 1)
					go func() {
						if state == "reading" {
							_, err := client.ReadFrame()
							pending <- err
						} else {
							pending <- client.WriteFrameContext(context.Background(), Frame{Kind: MessageHeartbeat})
						}
					}()
					synctest.Wait()
				}
				started := time.Now()
				err := client.Close()
				if elapsed := time.Since(started); elapsed != 0 || err != nil {
					t.Fatalf("close waited for remote TLS reader: elapsed=%s, error=%v", elapsed, err)
				}
				if pending != nil && <-pending == nil {
					t.Fatal("pending I/O succeeded after terminal close")
				}
				if _, err := server.ReadFrame(); err == nil {
					t.Fatal("remote reader did not observe closed connection")
				}
				if err := client.WriteFrameContext(context.Background(), Frame{Kind: MessageHeartbeat}); err == nil {
					t.Fatal("closed connection accepted another write")
				}
				if elapsed := time.Since(started); elapsed != 0 {
					t.Fatalf("terminal I/O remained blocked: %s", elapsed)
				}
			})
		})
	}
}
