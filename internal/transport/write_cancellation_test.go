package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestTimedOutTLSWriteClosesUnusableConnection(t *testing.T) {
	fixture := newTLSFixture(t)
	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	serverTLS := tls.Server(left, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots))
	clientTLS := tls.Client(right, mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
	_, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("authentication: %v / %v", serverErr, clientErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	frame := Frame{Kind: MessageHeartbeat, Payload: []byte{1}}
	if err := client.WriteFrameContext(ctx, frame); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("write: %v", err)
	}
	if err := client.WriteFrameContext(context.Background(), frame); err == nil {
		t.Fatal("reused TLS connection after failed write")
	}
}

type pausedWriteDeadline struct {
	started   chan struct{}
	release   chan struct{}
	deadlines []time.Time
}

func (writer *pausedWriteDeadline) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		close(writer.started)
		<-writer.release
	}
	writer.deadlines = append(writer.deadlines, deadline)
	return nil
}

func TestWriteCancellationJoinsCallbackBeforeReleasingNextWriter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		writer := &pausedWriteDeadline{started: make(chan struct{}), release: make(chan struct{})}
		finish := interruptWriteOnCancel(ctx, writer)
		cancel()
		<-writer.started
		finished := make(chan struct{})
		go func() { finish(); close(finished) }()
		synctest.Wait()
		select {
		case <-finished:
			t.Fatal("released writer while old deadline callback was pending")
		default:
		}
		close(writer.release)
		<-finished
		if len(writer.deadlines) != 2 || writer.deadlines[0].IsZero() || !writer.deadlines[1].IsZero() {
			t.Fatalf("deadline cleanup order: %v", writer.deadlines)
		}
	})
}

func TestSuccessfulWriteDisarmsCancellationBeforeNextWriter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		writer := &pausedWriteDeadline{started: make(chan struct{}), release: make(chan struct{})}
		finish := interruptWriteOnCancel(ctx, writer)
		finish()
		cancel()
		synctest.Wait()
		if len(writer.deadlines) != 1 || !writer.deadlines[0].IsZero() {
			t.Fatalf("late callback changed deadline: %v", writer.deadlines)
		}
	})
}
