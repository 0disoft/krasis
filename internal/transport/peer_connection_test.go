package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPeerConnectionExchangesFramesAfterAuthentication(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(
		t,
		mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots),
		mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName),
	)
	server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("peer authentication errors: server=%v client=%v", serverErr, clientErr)
	}

	want := Frame{Kind: MessageInput, Payload: []byte("input")}
	writeContext, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	defer cancelWrite()
	writeDone := make(chan error, 1)
	go func() { writeDone <- client.WriteFrameContext(writeContext, want) }()
	got, err := server.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	if got.Kind != want.Kind || !bytes.Equal(got.Payload, want.Payload) {
		t.Fatalf("ReadFrame() = %#v, want %#v", got, want)
	}
}

func TestPeerConnectionExposesAuthenticatedCertificateIdentity(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(
		t,
		mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots),
		mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName),
	)
	server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("peer authentication errors: server=%v client=%v", serverErr, clientErr)
	}
	defer server.Close()
	defer client.Close()
	serverPeer, err := server.AuthenticatedPeer()
	if err != nil {
		t.Fatal(err)
	}
	clientPeer, err := client.AuthenticatedPeer()
	if err != nil {
		t.Fatal(err)
	}
	if serverPeer.CertificateSHA256 != certificateFingerprint(fixture.clientIdentity.Certificate[0]) || serverPeer.Role != AuthenticatedPeerConnector {
		t.Fatalf("server peer = %+v", serverPeer)
	}
	if clientPeer.CertificateSHA256 != certificateFingerprint(fixture.serverIdentity.Certificate[0]) || clientPeer.Role != AuthenticatedPeerListener {
		t.Fatalf("client peer = %+v", clientPeer)
	}
}

func certificateFingerprint(der []byte) string {
	digest := sha256.Sum256(der)
	return hex.EncodeToString(digest[:])
}

func TestPeerConnectionRejectsMissingALPN(t *testing.T) {
	fixture := newTLSFixture(t)
	serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
	clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)
	clientConfig.NextProtos = []string{"other/1"}
	serverTLS, clientTLS := openTLSPair(t, serverConfig, clientConfig)
	server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if server != nil || client != nil {
		t.Fatal("peer wrapper was created without Krasis ALPN")
	}
	if !errors.Is(serverErr, ErrPeerAuthentication) || !errors.Is(clientErr, ErrPeerAuthentication) {
		t.Fatalf("authentication errors: server=%v client=%v", serverErr, clientErr)
	}
}

func TestPeerConnectionSerializesConcurrentWrites(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(
		t,
		mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots),
		mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName),
	)
	server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("peer authentication errors: server=%v client=%v", serverErr, clientErr)
	}

	const frameCount = 32
	writeContext, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	defer cancelWrite()
	readDone := make(chan error, 1)
	go func() {
		seen := make(map[byte]bool, frameCount)
		for range frameCount {
			frame, err := server.ReadFrame()
			if err != nil {
				readDone <- err
				return
			}
			if frame.Kind != MessageInput || len(frame.Payload) != 1 || seen[frame.Payload[0]] {
				readDone <- errors.New("concurrent frame was corrupted or duplicated")
				return
			}
			seen[frame.Payload[0]] = true
		}
		readDone <- nil
	}()
	var writers sync.WaitGroup
	writeErrors := make(chan error, frameCount)
	for index := range frameCount {
		writers.Add(1)
		go func(value byte) {
			defer writers.Done()
			if err := client.WriteFrameContext(writeContext, Frame{Kind: MessageInput, Payload: []byte{value}}); err != nil {
				writeErrors <- err
			}
		}(byte(index))
	}
	writers.Wait()
	close(writeErrors)
	for err := range writeErrors {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	if err := <-readDone; err != nil {
		t.Fatalf("concurrent read error = %v", err)
	}
}

func TestPeerConnectionWriteHonorsCanceledContextBeforeWriting(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(
		t,
		mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots),
		mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName),
	)
	server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
	if serverErr != nil || clientErr != nil {
		t.Fatalf("peer authentication errors: server=%v client=%v", serverErr, clientErr)
	}
	defer server.Close()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := client.WriteFrameContext(ctx, Frame{Kind: MessageHeartbeat, Payload: []byte{1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteFrameContext() error = %v, want context.Canceled", err)
	}
}

func authenticateConnectionPair(
	t *testing.T,
	serverTLS *tls.Conn,
	clientTLS *tls.Conn,
) (*PeerConnection, *PeerConnection, error, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type result struct {
		server     bool
		connection *PeerConnection
		err        error
	}
	done := make(chan result, 2)
	go func() {
		connection, err := NewPeerConnection(ctx, serverTLS)
		done <- result{server: true, connection: connection, err: err}
	}()
	go func() {
		connection, err := NewPeerConnection(ctx, clientTLS)
		done <- result{server: false, connection: connection, err: err}
	}()
	first := <-done
	second := <-done
	var server, client *PeerConnection
	var serverErr, clientErr error
	for _, outcome := range []result{first, second} {
		if outcome.server {
			server, serverErr = outcome.connection, outcome.err
		} else {
			client, clientErr = outcome.connection, outcome.err
		}
	}
	return server, client, serverErr, clientErr
}
