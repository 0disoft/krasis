package peerupdate

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuxiliaryConnectionLimitPrecedesTLSAndReleasesCapacity(t *testing.T) {
	certificate := httptest.NewTLSServer(http.NotFoundHandler())
	defer certificate.Close()
	listener, err := listenAuxiliary("127.0.0.1:0", certificate.TLS.Clone())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	m := &Manager{}
	var allowed atomic.Bool
	allowed.Store(true)
	var checks, active, peak atomic.Int32
	opened, closed := make(chan struct{}, 32), make(chan struct{}, 32)
	s := &http.Server{ReadHeaderTimeout: time.Minute, ErrorLog: log.New(io.Discard, "", 0), Handler: m.handler(strings.Repeat("a", 64), func() bool {
		checks.Add(1)
		return allowed.Load()
	}), ConnState: func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			n := active.Add(1)
			for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
			}
			opened <- struct{}{}
		case http.StateClosed:
			active.Add(-1)
			closed <- struct{}{}
		}
	}}
	done := make(chan error, 1)
	go func() { done <- s.Serve(listener) }()
	defer func() { _ = s.Close(); <-done }()
	var stalled []net.Conn
	defer func() {
		for _, c := range stalled {
			_ = c.Close()
		}
	}()
	for range maxAuxiliaryConnections {
		c, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		stalled = append(stalled, c)
		select {
		case <-opened:
		case <-time.After(2 * time.Second):
			t.Fatal("listener did not admit capacity")
		}
	}
	extra, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	_ = extra.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	_, err = extra.Read(b[:]) // No client sends a TLS ClientHello.
	var timeout net.Error
	if err == nil || errors.As(err, &timeout) && timeout.Timeout() {
		t.Error("overflow connection reached TLS instead of being closed", err)
	}
	if n := peak.Load(); n > maxAuxiliaryConnections || checks.Load() != 0 {
		t.Error("unauthenticated admission exceeded the limit or reached HTTP", n, checks.Load())
	}
	_ = extra.Close()
	for _, c := range stalled {
		_ = c.Close()
	}
	for range maxAuxiliaryConnections {
		select {
		case <-closed:
		case <-time.After(2 * time.Second):
			t.Fatal("closed connections retained capacity")
		}
	}
	client := certificate.Client()
	client.Timeout = 2 * time.Second
	defer client.CloseIdleConnections()
	url := "https://" + listener.Addr().String() + "/v1/app-open"
	for _, want := range []int{http.StatusNoContent, http.StatusForbidden} {
		r, err := client.Get(url)
		if err != nil {
			t.Fatal("released capacity did not admit HTTPS", err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Error("trust/status contract changed", r.StatusCode, want)
		}
		allowed.Store(false)
	}
	if checks.Load() != 2 {
		t.Fatal("per-request trust was bypassed", checks.Load())
	}
}

func TestAuxiliaryConnectionReleasesSlotOnlyOnce(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	c := &auxiliaryConnection{Conn: left, slots: slots}
	if err := c.Close(); err != nil || len(slots) != 0 {
		t.Fatal("connection did not release capacity", err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Error("repeated Close waited for or consumed another connection's slot")
		slots <- struct{}{} // Join a broken future implementation rather than leak a fixture.
		<-done
	}
}

func TestAuxiliaryListenerRejectsMissingTLSConfiguration(t *testing.T) {
	for _, config := range []*tls.Config{nil, {}} {
		listener, err := listenAuxiliary("127.0.0.1:0", config)
		if listener != nil {
			listener.Close()
		}
		if err == nil || listener != nil {
			t.Fatal("missing TLS identity opened a listener")
		}
	}
}
