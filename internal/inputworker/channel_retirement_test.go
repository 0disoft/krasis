package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestChannelCloseDoesNotWaitBehindPendingQueryResourceRetirement(t *testing.T) {
	server, pipe := net.Pipe()
	defer server.Close()
	channel := clientChannel(pipe, authorityService)
	entered, release := make(chan struct{}), make(chan struct{})
	retiring, finish := make(chan struct{}), make(chan struct{})
	channel.authority.checkInput = func() error { close(entered); <-release; return nil }
	channel.authority.closeHandles = func() error { close(retiring); <-finish; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- channel.authorizeInput(ctx, false) }()
	<-entered
	channel.queryMu.Lock()
	query := channel.query
	channel.queryMu.Unlock()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("query cancellation lost caller outcome", err)
	}
	close(release)
	<-retiring // Native query returned, but its resource owner has not retired.
	closed := make(chan error, 1)
	go func() { closed <- channel.Close() }()
	blocked := false
	select {
	case err := <-closed:
		if err != ErrAuthority {
			t.Error("unconfirmed retirement claimed success", err)
		}
	case <-time.After(100 * time.Millisecond):
		blocked = true
		t.Error("Close waited behind query-owned resource retirement")
	}
	close(finish)
	<-query.done
	if blocked {
		<-closed
	}
	if channel.Close() != nil {
		t.Fatal("finished resource retirement did not confirm cleanup")
	}
}
