package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestMutualTLSHandshake(t *testing.T) {
	fixture := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)

	serverState, clientState, serverErr, clientErr := handshakePair(t, serverConfig, clientConfig)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("handshake errors: server=%v client=%v", serverErr, clientErr)
	}
	for side, state := range map[string]tls.ConnectionState{"server": serverState, "client": clientState} {
		if state.Version != tls.VersionTLS13 {
			t.Fatalf("%s TLS version = %x, want TLS 1.3", side, state.Version)
		}
		if state.NegotiatedProtocol != ALPNProtocol {
			t.Fatalf("%s ALPN = %q, want %q", side, state.NegotiatedProtocol, ALPNProtocol)
		}
		if len(state.PeerCertificates) == 0 {
			t.Fatalf("%s has no authenticated peer certificate", side)
		}
	}
}

func TestMutualTLSRejectsMissingClientCertificate(t *testing.T) {
	fixture := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)
	clientConfig.Certificates = nil

	_, _, serverErr, clientErr := handshakePair(t, serverConfig, clientConfig)
	assertHandshakeRejected(t, serverErr, clientErr)
}

func TestMutualTLSRejectsUntrustedClient(t *testing.T) {
	fixture := newTLSFixture(t)
	untrusted := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, untrusted.clientIdentity, fixture.roots, fixture.serverName)

	_, _, serverErr, clientErr := handshakePair(t, serverConfig, clientConfig)
	assertHandshakeRejected(t, serverErr, clientErr)
}

func TestMutualTLSRejectsWrongServerName(t *testing.T) {
	fixture := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, "wrong-peer.test")

	_, _, serverErr, clientErr := handshakePair(t, serverConfig, clientConfig)
	assertHandshakeRejected(t, serverErr, clientErr)
}

func TestMutualTLSRejectsMissingKrasisALPN(t *testing.T) {
	fixture := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)
	clientConfig.NextProtos = []string{"other/1"}

	_, _, serverErr, clientErr := handshakePair(t, serverConfig, clientConfig)
	assertHandshakeRejected(t, serverErr, clientErr)
	if !errors.Is(serverErr, ErrPeerAuthentication) || !errors.Is(clientErr, ErrPeerAuthentication) {
		t.Fatalf("ALPN rejection errors: server=%v client=%v", serverErr, clientErr)
	}
}

func TestTLSConfigRequiresIdentityRootsAndExpectedName(t *testing.T) {
	fixture := newTLSFixture(t)
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "server identity", run: func() error { _, err := NewServerTLSConfig(tls.Certificate{}, fixture.roots); return err }},
		{name: "server roots", run: func() error { _, err := NewServerTLSConfig(fixture.serverIdentity, nil); return err }},
		{name: "client identity", run: func() error {
			_, err := NewClientTLSConfig(tls.Certificate{}, fixture.roots, fixture.serverName)
			return err
		}},
		{name: "client roots", run: func() error {
			_, err := NewClientTLSConfig(fixture.clientIdentity, nil, fixture.serverName)
			return err
		}},
		{name: "server name", run: func() error { _, err := NewClientTLSConfig(fixture.clientIdentity, fixture.roots, ""); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrInvalidTLSConfig) {
				t.Fatalf("error = %v, want ErrInvalidTLSConfig", err)
			}
		})
	}
}

func TestTLSConfigClonesTrustRoots(t *testing.T) {
	fixture := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)
	if serverConfig.ClientCAs == fixture.roots || clientConfig.RootCAs == fixture.roots {
		t.Fatal("TLS config retained caller-owned trust pool")
	}
	if serverConfig.ClientAuth != tls.RequireAndVerifyClientCert || clientConfig.InsecureSkipVerify {
		t.Fatal("TLS config weakened peer verification")
	}
}

type tlsFixture struct {
	roots          *x509.CertPool
	serverIdentity tls.Certificate
	clientIdentity tls.Certificate
	serverName     string
}

func newTLSFixture(t *testing.T) tlsFixture {
	t.Helper()
	now := time.Now()
	caKey := newPrivateKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Krasis test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCertificate)

	serverName := "peer.test"
	serverIdentity := issueIdentity(t, caCertificate, caKey, big.NewInt(2), serverName, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	clientIdentity := issueIdentity(t, caCertificate, caKey, big.NewInt(3), "client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
	return tlsFixture{
		roots:          roots,
		serverIdentity: serverIdentity,
		clientIdentity: clientIdentity,
		serverName:     serverName,
	}
}

func issueIdentity(
	t *testing.T,
	ca *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	serial *big.Int,
	commonName string,
	usage []x509.ExtKeyUsage,
) tls.Certificate {
	t.Helper()
	key := newPrivateKey(t)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     []string{commonName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usage,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("issue identity certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse identity certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key, Leaf: leaf}
}

func newPrivateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	return key
}

func mustServerTLSConfig(t *testing.T, identity tls.Certificate, roots *x509.CertPool) *tls.Config {
	t.Helper()
	config, err := NewServerTLSConfig(identity, roots)
	if err != nil {
		t.Fatalf("NewServerTLSConfig() error = %v", err)
	}
	return config
}

func mustClientTLSConfig(
	t *testing.T,
	identity tls.Certificate,
	roots *x509.CertPool,
	serverName string,
) *tls.Config {
	t.Helper()
	config, err := NewClientTLSConfig(identity, roots, serverName)
	if err != nil {
		t.Fatalf("NewClientTLSConfig() error = %v", err)
	}
	return config
}

func handshakePair(
	t *testing.T,
	serverConfig *tls.Config,
	clientConfig *tls.Config,
) (tls.ConnectionState, tls.ConnectionState, error, error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}
	acceptDone := make(chan struct {
		connection net.Conn
		err        error
	}, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		acceptDone <- struct {
			connection net.Conn
			err        error
		}{connection: connection, err: acceptErr}
	}()
	clientSide, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("dial loopback: %v", err)
	}
	accepted := <-acceptDone
	_ = listener.Close()
	if accepted.err != nil {
		_ = clientSide.Close()
		t.Fatalf("accept loopback: %v", accepted.err)
	}
	serverSide := accepted.connection
	server := tls.Server(serverSide, serverConfig)
	client := tls.Client(clientSide, clientConfig)
	t.Cleanup(func() {
		_ = serverSide.Close()
		_ = clientSide.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type result struct {
		server bool
		err    error
	}
	done := make(chan result, 2)
	go func() { done <- result{server: true, err: AuthenticatePeer(ctx, server)} }()
	go func() { done <- result{server: false, err: AuthenticatePeer(ctx, client)} }()
	first := <-done
	if first.err != nil {
		_ = serverSide.Close()
		_ = clientSide.Close()
	}
	second := <-done
	var serverErr, clientErr error
	for _, outcome := range []result{first, second} {
		if outcome.server {
			serverErr = outcome.err
		} else {
			clientErr = outcome.err
		}
	}
	return server.ConnectionState(), client.ConnectionState(), serverErr, clientErr
}

func assertHandshakeRejected(t *testing.T, serverErr, clientErr error) {
	t.Helper()
	if serverErr == nil && clientErr == nil {
		t.Fatal("unauthenticated handshake succeeded")
	}
	for side, err := range map[string]error{"server": serverErr, "client": clientErr} {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			t.Fatalf("%s handshake timed out instead of rejecting authentication: server=%v client=%v", side, serverErr, clientErr)
		}
	}
}
