//go:build windows

package transport

import (
	"net"
	"testing"
)

func TestAuthenticatedInputConnectionRequestsACKFrequency(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(t,
		mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots),
		mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
	server, client, se, ce := authenticateConnectionPair(t, serverTLS, clientTLS)
	if se != nil || ce != nil {
		t.Fatalf("authentication: %v / %v", se, ce)
	}
	defer server.Close()
	defer client.Close()
	for _, connection := range []*PeerConnection{server, client} {
		if got := connection.ACKPolicy(); got != "tcp_ack_every_packet_requested" {
			t.Fatalf("native loopback request not accepted on this test host: %s", got)
		}
	}
}

func TestACKFrequencyFailureDoesNotInventSuccessOrCloseStream(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if got := configureConnectionACK(a); got != "tcp_ack_frequency_not_tcp" {
		t.Fatal(got)
	}
	if got := configureConnectionACK(&net.TCPConn{}); got != "tcp_ack_frequency_unavailable" {
		t.Fatal(got)
	}
	if (*PeerConnection)(nil).ACKPolicy() != "tcp_ack_frequency_unavailable" ||
		(&PeerConnection{}).ACKPolicy() != "tcp_ack_frequency_unavailable" {
		t.Fatal("unknown connection claimed an accepted request")
	}
	// Unsupported streams stay owned by their caller and remain usable.
	go func() { _, _ = a.Write([]byte{1}) }()
	var data [1]byte
	if n, err := b.Read(data[:]); err != nil || n != 1 || data[0] != 1 {
		t.Fatalf("unsupported hint damaged caller-owned stream: %d %v", n, err)
	}
}
