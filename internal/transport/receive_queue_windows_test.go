//go:build windows

package transport

import (
	"net"
	"testing"
	"time"
)

func TestSocketReceiveQueueDoesNotConsumeChangeOrOwnSocket(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if n, ok := socketReceiveQueue(server); !ok || n != 0 {
		t.Fatal("empty live socket", n, ok)
	}
	if _, err := client.Write([]byte("queued bytes")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		n, ok := socketReceiveQueue(server)
		if !ok {
			t.Fatal("live socket FIONREAD unavailable")
		}
		if n == 12 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("loopback bytes not received", n)
		}
		time.Sleep(time.Millisecond)
	}
	if err := server.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	var data [12]byte
	if n, err := server.Read(data[:]); err != nil || n != 12 || string(data[:]) != "queued bytes" {
		t.Fatal("probe consumed bytes", n, err)
	}
	server.Close()
	if _, ok := socketReceiveQueue(server); ok {
		t.Fatal("closed socket became zero evidence")
	}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, ok := socketReceiveQueue(a); ok {
		t.Fatal("non-TCP stream became queue evidence")
	}
}
