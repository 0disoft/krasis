package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestDialSourceBoundsSilentTLSAndClosesSocket(t *testing.T) {
	testSilentDialCleanup(t, false)
}

func TestDialSourceCancellationClosesSilentTLSSocket(t *testing.T) {
	testSilentDialCleanup(t, true)
}

func testSilentDialCleanup(t *testing.T, cancelDuringTLS bool) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	started := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			closed <- err
			return
		}
		defer connection.Close()
		// Consume the ClientHello but never send a ServerHello. EOF proves
		// the client retired its socket, not just the Next caller's wait.
		var first [1]byte
		_, err = io.ReadFull(connection, first[:])
		if err == nil {
			close(started)
			_, err = io.Copy(io.Discard, connection)
		}
		closed <- err
	}()
	budget := 250 * time.Millisecond
	if cancelDuringTLS {
		budget = time.Second
	}
	source, err := NewDialSource(listener.Addr().String(), &tls.Config{ServerName: "localhost"}, budget)
	if err != nil {
		t.Fatal(err)
	}
	// This outer deadline only bounds a broken implementation. A stalled
	// handshake must use the source's budget even without a caller deadline.
	parent, stopParent := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopParent()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := context.AfterFunc(parent, cancel)
	defer stop()
	result := make(chan error, 1)
	go func() {
		connection, err := source.Next(ctx)
		if connection != nil {
			_ = connection.Close()
		}
		result <- err
	}()
	select {
	case <-started:
	case <-parent.Done():
		t.Fatal("ClientHello was not sent")
	}
	want := context.DeadlineExceeded
	if cancelDuringTLS {
		cancel()
		want = context.Canceled
	}
	err = <-result
	if !errors.Is(err, want) {
		t.Fatalf("Next error = %v, want %v", err, want)
	}
	if parent.Err() != nil {
		t.Fatal("TLS handshake retained the socket until the caller's safety deadline")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("server did not observe a clean socket retirement: %v", err)
		}
	case <-parent.Done():
		t.Fatal("Next returned without closing the unauthenticated socket")
	}
}
