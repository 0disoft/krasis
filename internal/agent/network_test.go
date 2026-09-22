package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
)

func TestListenSourceClosePreservesFirstError(t *testing.T) {
	closeErr := errors.New("listener close failed")
	listener := &closeErrorListener{closeErr: closeErr}
	source := &ListenSource{listener: listener, tlsConfig: &tls.Config{}}
	if err := source.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := source.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("second Close() error = %v", err)
	}
	if listener.closeCalls != 1 {
		t.Fatalf("listener close calls = %d", listener.closeCalls)
	}
}

func TestCloseConnectionOnErrorPreservesCauseAndCloseFailure(t *testing.T) {
	cause := errors.New("authentication failed")
	closeErr := errors.New("socket close failed")
	connection := &closeErrorConn{closeErr: closeErr}
	err := closeConnectionOnError(connection, cause)
	if !errors.Is(err, cause) || !errors.Is(err, closeErr) {
		t.Fatalf("closeConnectionOnError() error = %v", err)
	}
	if connection.closeCalls != 1 {
		t.Fatalf("connection close calls = %d", connection.closeCalls)
	}
}

func TestListenSourceRejectsUnspecifiedNetworkPolicyBeforeBinding(t *testing.T) {
	_, err := NewListenSourceWithPolicy(
		"127.0.0.1:0",
		&tls.Config{},
		"",
		DefaultPreAuthAdmissionConfig(),
	)
	if err == nil {
		t.Fatal("expected explicit network policy error")
	}
}

func TestListenAndDialSourcesAuthenticatePeerConnection(t *testing.T) {
	testListenAndDialSourcesAuthenticatePeerConnection(t, false)
}

func TestListenSourceRecoversTemporaryAcceptFailure(t *testing.T) {
	testListenAndDialSourcesAuthenticatePeerConnection(t, true)
}

func testListenAndDialSourcesAuthenticatePeerConnection(t *testing.T, failOnce bool) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "credentials")
	if _, err := peercredentials.Generate(directory, "127.0.0.1", time.Now()); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	roots := loadAgentRoots(t, filepath.Join(directory, "listener", "ca.pem"))
	listenerIdentity := loadAgentIdentity(t, filepath.Join(directory, "listener"))
	connectorIdentity := loadAgentIdentity(t, filepath.Join(directory, "connector"))
	serverTLS, err := transport.NewServerTLSConfig(listenerIdentity, roots)
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := transport.NewClientTLSConfig(connectorIdentity, roots, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	socket, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = socket.Close() })
	var underlying net.Listener = socket
	if failOnce {
		underlying = &temporaryAcceptListener{Listener: socket}
	}
	listener, err := newListenSource(underlying, serverTLS, time.Second, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if listener.NetworkPolicy() != ListenNetworkTrustedPrivate {
		t.Fatalf("network policy = %q", listener.NetworkPolicy())
	}
	dialer, err := NewDialSource(listener.Address(), clientTLS, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		connection Connection
		err        error
	}
	accepted := make(chan result, 1)
	go func() { connection, nextErr := listener.Next(ctx); accepted <- result{connection, nextErr} }()
	waitForListenRound(t, listener)
	clientConnection, err := dialer.Next(ctx)
	if err != nil {
		t.Fatalf("DialSource.Next() error = %v", err)
	}
	serverResult := <-accepted
	if serverResult.err != nil {
		t.Fatalf("ListenSource.Next() error = %v", serverResult.err)
	}
	_ = clientConnection.Close()
	_ = serverResult.connection.Close()
	metrics := listener.PreAuthMetrics()
	if metrics.Admitted != 1 || metrics.Authenticated != 1 {
		t.Fatalf("pre-auth metrics = %+v", metrics)
	}
}

type temporaryAcceptError struct{}

func (temporaryAcceptError) Error() string   { return "temporary accept failure" }
func (temporaryAcceptError) Temporary() bool { return true }

type temporaryAcceptListener struct {
	net.Listener
	failed bool
}

type repeatedTemporaryListener struct {
	closeErrorListener
	attempts int
}

func (listener *repeatedTemporaryListener) Accept() (net.Conn, error) {
	listener.attempts++
	return nil, temporaryAcceptError{}
}

func TestListenSourceCloseInterruptsTemporaryErrorBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := &repeatedTemporaryListener{}
		source, err := newListenSource(listener, &tls.Config{}, time.Second, 1)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		before := time.Now()
		if err := source.Close(); err != nil {
			t.Fatal(err)
		}
		if listener.attempts < 2 || listener.attempts > 12 {
			t.Fatalf("unbounded retry loop: %d", listener.attempts)
		}
		if time.Since(before) != 0 || listener.closeCalls != 1 {
			t.Fatal("shutdown did not interrupt backoff or closed listener twice")
		}
	})
}
func (listener *temporaryAcceptListener) Accept() (net.Conn, error) {
	if !listener.failed {
		listener.failed = true
		return nil, temporaryAcceptError{}
	}
	return listener.Listener.Accept()
}

func TestListenSourceTerminalAcceptFailureRemainsExplicit(t *testing.T) {
	source, err := newListenSource(&closeErrorListener{}, &tls.Config{}, time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for range 2 {
		if _, err := source.Next(ctx); err == nil || err.Error() != "unused" {
			t.Fatalf("terminal error lost: %v", err)
		}
	}
}

func TestListenSourceDoesNotLetStalledHandshakeBlockAuthenticatedPeer(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "credentials")
	if _, err := peercredentials.Generate(directory, "127.0.0.1", time.Now()); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	roots := loadAgentRoots(t, filepath.Join(directory, "listener", "ca.pem"))
	listenerIdentity := loadAgentIdentity(t, filepath.Join(directory, "listener"))
	connectorIdentity := loadAgentIdentity(t, filepath.Join(directory, "connector"))
	serverTLS, err := transport.NewServerTLSConfig(listenerIdentity, roots)
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := transport.NewClientTLSConfig(connectorIdentity, roots, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := NewListenSource("127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	type result struct {
		connection Connection
		err        error
	}
	accepted := make(chan result, 1)
	go func() {
		connection, nextErr := listener.Next(ctx)
		accepted <- result{connection: connection, err: nextErr}
	}()
	waitForListenRound(t, listener)

	stalled, err := net.DialTimeout("tcp", listener.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close()
	if _, err := stalled.Write([]byte{0x16}); err != nil {
		t.Fatal(err)
	}

	dialer, err := NewDialSource(listener.Address(), clientTLS, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clientConnection, err := dialer.Next(ctx)
	if err != nil {
		t.Fatalf("authenticated peer was blocked by stalled handshake: %v", err)
	}
	defer clientConnection.Close()
	serverResult := <-accepted
	if serverResult.err != nil {
		t.Fatalf("ListenSource.Next() error = %v", serverResult.err)
	}
	defer serverResult.connection.Close()
	metrics := listener.PreAuthMetrics()
	if metrics.Admitted != 2 || metrics.Authenticated != 1 {
		t.Fatalf("pre-auth metrics = %+v", metrics)
	}
}

func waitForListenRound(t *testing.T, source *ListenSource) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		source.roundMu.Lock()
		pending := source.round != nil
		source.roundMu.Unlock()
		if pending {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("ListenSource.Next() did not register an accept round")
}

func loadAgentIdentity(t *testing.T, directory string) tls.Certificate {
	t.Helper()
	identity, err := tls.LoadX509KeyPair(filepath.Join(directory, "identity.pem"), filepath.Join(directory, "identity-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func loadAgentRoots(t *testing.T, path string) *x509.CertPool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		t.Fatal("append CA")
	}
	return roots
}

type closeErrorListener struct {
	closeCalls int
	closeErr   error
}

func (*closeErrorListener) Accept() (net.Conn, error) { return nil, errors.New("unused") }
func (listener *closeErrorListener) Close() error {
	listener.closeCalls++
	return listener.closeErr
}
func (*closeErrorListener) Addr() net.Addr { return nil }

type closeErrorConn struct {
	closeCalls int
	closeErr   error
}

func (*closeErrorConn) Read([]byte) (int, error)         { return 0, errors.New("unused") }
func (*closeErrorConn) Write([]byte) (int, error)        { return 0, errors.New("unused") }
func (connection *closeErrorConn) Close() error          { connection.closeCalls++; return connection.closeErr }
func (*closeErrorConn) LocalAddr() net.Addr              { return nil }
func (*closeErrorConn) RemoteAddr() net.Addr             { return nil }
func (*closeErrorConn) SetDeadline(time.Time) error      { return nil }
func (*closeErrorConn) SetReadDeadline(time.Time) error  { return nil }
func (*closeErrorConn) SetWriteDeadline(time.Time) error { return nil }
