package peerupdate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Closing a listener never closes connections that Accept already returned.
type auxiliaryFaultListener struct {
	connections []net.Conn
	next        int
	fail        bool
	closed      chan struct{}
	once        sync.Once
}

func (l *auxiliaryFaultListener) Accept() (net.Conn, error) {
	if l.next < len(l.connections) {
		c := l.connections[l.next]
		l.next++
		return c, nil
	}
	if l.fail {
		return nil, errors.New("permanent accept failure")
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *auxiliaryFaultListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *auxiliaryFaultListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestAuxiliaryServeClosesAcceptedSocketsOnAcceptFailureAndCancellation(t *testing.T) {
	for _, failAccept := range []bool{true, false} {
		name := "cancel"
		if failAccept {
			name = "accept failure"
		}
		t.Run(name, func(t *testing.T) {
			listener := &auxiliaryFaultListener{fail: failAccept, closed: make(chan struct{})}
			slots := make(chan struct{}, maxAuxiliaryConnections)
			for range maxAuxiliaryConnections {
				serverSide, clientSide := net.Pipe()
				t.Cleanup(func() { _ = clientSide.Close() })
				slots <- struct{}{}
				connection := &auxiliaryConnection{Conn: serverSide, slots: slots}
				listener.connections = append(listener.connections, connection)
				t.Cleanup(func() { _ = connection.Close() })
			}
			admitted := make(chan struct{}, maxAuxiliaryConnections)
			server := &http.Server{ReadHeaderTimeout: time.Hour, ConnState: func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					admitted <- struct{}{}
				}
			}}
			t.Cleanup(func() { _ = server.Close() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { defer close(done); serveAuxiliary(ctx, server, listener) }()
			for range maxAuxiliaryConnections {
				select {
				case <-admitted:
				case <-time.After(2 * time.Second):
					t.Fatal("server did not accept the connection capacity")
				}
			}
			if !failAccept {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("server did not exit")
			}
			if got := len(slots); got != 0 {
				t.Fatalf("server exit retained %d accepted sockets and connection slots", got)
			}
			select {
			case <-listener.closed:
			default:
				t.Fatal("server exit retained its listener")
			}
		})
	}
}
