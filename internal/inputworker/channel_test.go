package inputworker

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

type failingCloseConn struct {
	net.Conn
	closes atomic.Int32
}

func (c *failingCloseConn) Close() error {
	c.closes.Add(1)
	_ = c.Conn.Close()
	return errors.New("close failed")
}

func TestChannelConcurrentClosePreservesFirstFailure(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &failingCloseConn{Conn: conn}
	channel := clientChannel(stream, authorityService)
	var closes atomic.Int32
	channel.authority.closeHandles = func() error { closes.Add(1); return nil }
	var callers sync.WaitGroup
	for i := 0; i < 10; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := channel.Close(); !errors.Is(err, ErrAuthority) {
				t.Error("close discarded earlier failure", err)
			}
		}()
	}
	callers.Wait()
	if stream.closes.Load() != 1 || closes.Load() != 1 {
		t.Fatal("channel resources closed more than once")
	}
}

func TestChannelCloseRetainsPreviouslyFailedProofCleanup(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	channel := clientChannel(conn, authorityService)
	closes := 0
	channel.authority.closeHandles = func() error { closes++; return ErrAuthority }
	if err := channel.authority.Close(); err != ErrAuthority {
		t.Fatal("failed native proof cleanup was accepted", err)
	}
	if err := channel.Close(); err != ErrAuthority || closes != 1 {
		t.Fatal("channel disguised an earlier proof cleanup failure", closes, err)
	}
}
