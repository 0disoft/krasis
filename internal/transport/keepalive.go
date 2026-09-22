package transport

import (
	"net"
	"time"
)

// Idle TCP probes detect abandoned LOCAL connections independently of the
// active lease heartbeat. They neither renew a lease nor authorize input.
const TCPKeepAlivePolicy = "tcp_keepalive_15_5_3"

func configureConnectionKeepAlive(connection net.Conn) string {
	tcp, ok := connection.(*net.TCPConn)
	if !ok {
		return "tcp_keepalive_not_tcp"
	}
	if err := tcp.SetKeepAliveConfig(net.KeepAliveConfig{Enable: true, Idle: 15 * time.Second, Interval: 5 * time.Second, Count: 3}); err != nil {
		// Do not describe a rejected or partially applied setting as disabled.
		return "tcp_keepalive_unavailable"
	}
	return TCPKeepAlivePolicy
}

func (connection *PeerConnection) KeepAlivePolicy() string {
	if connection == nil {
		return "tcp_keepalive_unavailable"
	}
	return connection.keepAlivePolicy
}
