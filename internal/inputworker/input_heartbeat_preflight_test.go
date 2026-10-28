package inputworker

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestInputHeartbeatPreflightDoesNotBlockFreshInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
		channel := clientChannel(conn, authorityService)
		client, err := NewClient(context.Background(), channel, testLease())
		if err != nil {
			t.Fatal(err)
		}
		entered, release := make(chan struct{}), make(chan struct{})
		var checks atomic.Int32
		channel.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				close(entered)
				<-release
			}
			return nil
		}
		heartbeat := make(chan error, 1)
		go func() { heartbeat <- client.Heartbeat(context.Background()) }()
		<-entered
		moved := make(chan error, 1)
		go func() { moved <- client.Move(context.Background(), 1, 1) }()
		synctest.Wait()
		select {
		case err := <-moved:
			if err != nil {
				t.Error("fresh input failed during maintenance", err)
			}
		default:
			t.Error("unsent heartbeat preflight owns the input gate")
		}
		close(release)
		if err := <-heartbeat; err != nil {
			t.Error(err)
		}
		synctest.Wait()
		if client.sequence != 2 {
			t.Error("maintenance ignored the fresh input ACK", client.sequence)
		}
		_ = client.Close()
		if result := await(t, done); result.err != nil || result.report.Completed != 1 {
			t.Error(result)
		}
	})
}

func TestInputHeartbeatSentFailureStillRetiresChannel(t *testing.T) {
	for _, missing := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &admissionObservedConn{Conn: conn}
			channel := clientChannel(stream, authorityService)
			client, err := NewClient(context.Background(), channel, testLease())
			if err != nil {
				t.Fatal(err)
			}
			worker := make(chan struct{})
			go func() {
				defer close(worker)
				command, err := readRequest(server)
				if err != nil {
					t.Error(err)
					return
				}
				if !missing {
					_ = writeResponse(server, response{sequence: command.sequence + 1, op: command.op})
				} else {
					// The sent request's original two-second ACK deadline remains fatal.
					var b [1]byte
					_, _ = server.Read(b[:])
				}
			}()
			if err := client.Heartbeat(context.Background()); err == nil || !client.failed || stream.writes.Load() != 1 {
				t.Error("lost/mismatched sent ACK became a soft timeout", err)
			}
			if err := client.Heartbeat(context.Background()); err == nil || stream.writes.Load() != 1 {
				t.Error("failed heartbeat was replayed", err)
			}
			_ = client.Close()
			<-worker
		})
	}
}

func TestInputHeartbeatCannotUsePresenceAfterAuthorityFenced(t *testing.T) {
	conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
	stream := &admissionObservedConn{Conn: conn}
	channel := clientChannel(stream, authorityService)
	client, err := NewClient(context.Background(), channel, testLease())
	if err != nil {
		t.Fatal(err)
	}
	channel.authority.checkPresent = func() error { channel.authority.fenced.Store(true); return nil }
	if err := client.Heartbeat(context.Background()); err == nil || stream.writes.Load() != 0 || client.sequence != 1 {
		t.Error("presence success revived a fenced channel", err)
	}
	_ = client.Close()
	_ = await(t, done)
}

func TestInputHeartbeatUnsentTimeoutDoesNotFenceInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
		channel := clientChannel(conn, authorityService)
		client, err := NewClient(context.Background(), channel, testLease())
		if err != nil {
			t.Fatal(err)
		}
		release := make(chan struct{})
		var checks atomic.Int32
		channel.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				<-release
			}
			return nil
		}
		// Always join the native fixture, including against the old code.
		timer := time.AfterFunc(3*time.Second, func() { close(release) })
		started := time.Now()
		err = client.Heartbeat(context.Background())
		if err != nil || time.Since(started) != time.Second || client.sequence != 1 || channel.authority.fenced.Load() || client.failed {
			t.Error("unsent maintenance timeout retired live input", err, time.Since(started))
		}
		if timer.Stop() {
			close(release)
		}
		synctest.Wait()
		if err := client.Move(context.Background(), 1, 1); err != nil {
			t.Error("live input did not recover", err)
		}
		_ = client.Close()
		if result := await(t, done); result.err != nil || result.report.Completed != 1 {
			t.Error(result)
		}
	})
}
