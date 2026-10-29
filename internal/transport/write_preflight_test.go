package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type delayedDeadlineConn struct {
	net.Conn
	delay atomic.Bool
}

func (c *delayedDeadlineConn) SetWriteDeadline(deadline time.Time) error {
	err := c.Conn.SetWriteDeadline(deadline)
	if !deadline.IsZero() && c.delay.CompareAndSwap(true, false) {
		time.Sleep(30 * time.Millisecond)
	}
	return err
}

func TestExpiryDuringDeadlineSetupDoesNotPoisonUntouchedTLS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newTLSFixture(t)
		left, right := net.Pipe()
		defer left.Close()
		defer right.Close()
		delayed := &delayedDeadlineConn{Conn: right}
		serverTLS := tls.Server(left, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots))
		clientTLS := tls.Client(delayed, mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
		server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
		if serverErr != nil || clientErr != nil {
			t.Fatal(serverErr, clientErr)
		}
		delayed.delay.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		frame := Frame{Kind: MessageControl, Payload: []byte{1}}
		err := client.WriteFrameContext(ctx, frame)
		var failure *FrameWriteFailure
		if !errors.As(err, &failure) || failure.Phase != "admission" || failure.IO != 0 || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("untouched stream classified as failed TLS I/O: %+v, %v", failure, err)
		}
		read := make(chan error, 1)
		go func() {
			got, err := server.ReadFrame()
			if err == nil && (got.Kind != MessageHeartbeat || len(got.Payload) != 1 || got.Payload[0] != 2) {
				err = errors.New("expired frame reached the stream")
			}
			read <- err
		}()
		if err := client.WriteFrameContext(context.Background(), Frame{Kind: MessageHeartbeat, Payload: []byte{2}}); err != nil {
			t.Fatal("untouched TLS stream was poisoned", err)
		}
		if err := <-read; err != nil {
			t.Fatal(err)
		}
	})
}
