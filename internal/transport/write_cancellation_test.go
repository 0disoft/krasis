package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestTimedOutTLSWriteClosesUnusableConnection(t *testing.T) {
	for _, kind := range []MessageKind{MessageControl, MessageInput, MessageHeartbeat, MessageDisconnect} {
		for _, callerLimit := range []time.Duration{0, 20 * time.Millisecond} {
			t.Run(fmt.Sprintf("kind-%d/%s", kind, callerLimit), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					fixture := newTLSFixture(t)
					left, right := net.Pipe()
					t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
					serverTLS := tls.Server(left, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots))
					clientTLS := tls.Client(right, mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
					_, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
					if serverErr != nil || clientErr != nil {
						t.Fatalf("authentication: %v / %v", serverErr, clientErr)
					}
					ctx := context.Background()
					expected := FrameWriteTimeout
					if callerLimit != 0 {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, callerLimit)
						defer cancel()
						expected = callerLimit
					}
					frame := Frame{Kind: kind, Payload: []byte{1}}
					started := time.Now()
					err := client.WriteFrameContext(ctx, frame)
					var failure *FrameWriteFailure
					if !errors.As(err, &failure) || failure.Phase != "io" || failure.Kind != kind || failure.IO != expected || time.Since(started) != expected {
						t.Fatalf("write deadline: %v, detail=%+v, elapsed=%s", err, failure, time.Since(started))
					}
					if err := client.WriteFrameContext(context.Background(), frame); err == nil {
						t.Fatal("reused TLS connection after failed write")
					}
				})
			})
		}
	}
}

func TestWriteAdmissionTimeoutDoesNotCloseCurrentOwnersConnection(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(t, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots), mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
	server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("authentication: %v / %v", serverErr, clientErr)
	}
	<-client.writeGate
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	frame := Frame{Kind: MessageHeartbeat, Payload: []byte{1}}
	err := client.WriteFrameContext(ctx, frame)
	client.writeGate <- struct{}{}
	var failure *FrameWriteFailure
	if !errors.As(err, &failure) || failure.Phase != "admission" || failure.IO != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("admission: %+v, %v", failure, err)
	}
	if err := client.WriteFrameContext(context.Background(), frame); err != nil {
		t.Fatalf("waiting writer closed untouched stream: %v", err)
	}
	if received, err := server.ReadFrame(); err != nil || received.Kind != frame.Kind {
		t.Fatalf("next frame: %+v, %v", received, err)
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
