//go:build windows

package transport

import (
	"net"
	"testing"
)

func TestAuthenticatedSocketTCPInfoQueriesTheLiveWindowsSocket(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(t, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots), mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
	server, client, se, ce := authenticateConnectionPair(t, serverTLS, clientTLS)
	if se != nil || ce != nil {
		t.Fatalf("authentication: %v / %v", se, ce)
	}
	for _, peer := range []*PeerConnection{server, client} {
		info, ok := peer.DiagnosticTCPInfo()
		if !ok {
			t.Fatal("SIO_TCP_INFO v0 failed on authenticated TCP socket")
		}
		t.Logf("live TCP estimated/minimum RTT: %d/%d us", info[0], info[1])
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.DiagnosticTCPInfo(); ok {
		t.Fatal("closed socket became statistics")
	}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, ok := socketTCPInfo(a); ok {
		t.Fatal("non-TCP stream became statistics")
	}
}
