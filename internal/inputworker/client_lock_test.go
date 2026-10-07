package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestClientCanceledLockWaitDoesNotSendOrRetireActiveOperation(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	entered, release, workerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	go func() {
		defer close(workerDone)
		defer server.Close()
		first, err := readRequest(server)
		if err != nil {
			return
		}
		reads.Add(1)
		close(entered)
		<-release
		if writeResponse(server, response{sequence: first.sequence, op: first.op}) != nil {
			return
		}
		second, err := readRequest(server)
		if err != nil {
			return
		}
		reads.Add(1)
		_ = writeResponse(server, response{sequence: second.sequence, op: second.op})
	}()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	firstDone := make(chan error, 1)
	go func() { firstDone <- client.Move(context.Background(), 1, 0) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	secondDone := make(chan error, 1)
	go func() { secondDone <- client.Move(ctx, 2, 0) }()
	select {
	case err := <-secondDone:
		certainty, ok := application.InjectionCertaintyOf(err)
		if !errors.Is(err, context.DeadlineExceeded) || !ok || certainty != application.InjectionNotApplied {
			t.Fatal("queued input lost its not-sent certainty", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued input remained blocked")
	}
	if reads.Load() != 1 {
		t.Fatal("canceled waiter sent input")
	}
	select {
	case err := <-firstDone:
		t.Fatal("waiter retired active owner's operation", err)
	default:
	}
	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("active operation did not finish")
	}
	if err := client.Move(context.Background(), 3, 0); err != nil {
		t.Fatal("canceled wait damaged sequence or channel", err)
	}
	<-workerDone
	if reads.Load() != 2 {
		t.Fatal("canceled input was replayed")
	}
}
