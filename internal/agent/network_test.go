package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
)

func TestListenAndDialSourcesAuthenticatePeerConnection(t *testing.T) {
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
