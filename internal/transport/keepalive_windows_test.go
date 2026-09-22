//go:build windows

package transport

import (
	"net"
	"syscall"
	"testing"
)

func TestAuthenticatedTCPKeepAlivePolicyMatchesWindowsSocket(t *testing.T) {
	fixture := newTLSFixture(t)
	serverTLS, clientTLS := openTLSPair(t, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots), mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
	server, client, se, ce := authenticateConnectionPair(t, serverTLS, clientTLS)
	if se != nil || ce != nil {
		t.Fatalf("authentication: %v / %v", se, ce)
	}
	for _, peer := range []*PeerConnection{server, client} {
		if peer.KeepAlivePolicy() != TCPKeepAlivePolicy {
			t.Fatalf("policy unavailable: %s", peer.KeepAlivePolicy())
		}
		socket, err := peer.connection.NetConn().(*net.TCPConn).SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		if err := socket.Control(func(fd uintptr) {
			// Windows TCP option identifiers match Go's internal/syscall/windows.
			for _, option := range []struct{ level, key, want int }{{syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1}, {syscall.IPPROTO_TCP, 0x03, 15}, {syscall.IPPROTO_TCP, 0x11, 5}, {syscall.IPPROTO_TCP, 0x10, 3}} {
				got, err := syscall.GetsockoptInt(syscall.Handle(fd), option.level, option.key)
				if err != nil || got != option.want {
					t.Errorf("socket policy option %d=%d want %d: %v", option.key, got, option.want, err)
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnsupportedKeepAliveIsNeverReportedAsDisabledOrEnabled(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if got := configureConnectionKeepAlive(a); got != "tcp_keepalive_not_tcp" {
		t.Fatal(got)
	}
	if got := configureConnectionKeepAlive(&net.TCPConn{}); got != "tcp_keepalive_unavailable" {
		t.Fatal(got)
	}
}
