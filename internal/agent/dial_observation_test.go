package agent

import (
	"context"
	"crypto/tls"
	"github.com/0disoft/krasis/internal/netobserve"
	"net"
	"testing"
	"time"
)

func TestDialObserverSeparatesTCPAndTLSFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	address := listener.Addr().String()
	dial, err := NewDialSource(address, &tls.Config{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var got []netobserve.DialAttempt
	dial.ObserveDial = func(a netobserve.DialAttempt) { got = append(got, a) }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if conn, err := dial.Next(ctx); err == nil {
		conn.Close()
		t.Fatal("unauthenticated socket accepted")
	}
	listener.Close()
	<-done
	if len(got) != 2 || got[0].Phase != "tcp" || got[0].Err != nil || got[0].Local == "" || got[1].Phase != "tls_auth" || got[1].Err == nil {
		t.Fatalf("%+v", got)
	}
	got = nil
	if conn, err := dial.Next(ctx); err == nil {
		conn.Close()
		t.Fatal("closed listener accepted")
	}
	if len(got) != 1 || got[0].Phase != "tcp" || got[0].Err == nil || got[0].Local != "" {
		t.Fatalf("%+v", got)
	}
}
