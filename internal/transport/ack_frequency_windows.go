//go:build windows

package transport

import (
	"net"

	"golang.org/x/sys/windows"
)

// Apply once to the authenticated input socket, never to auxiliary transfers.
// This is a best-effort socket hint, not evidence of packet delivery or latency.
func configureConnectionACK(connection net.Conn) string {
	tcp, ok := connection.(*net.TCPConn)
	if !ok {
		return "tcp_ack_frequency_not_tcp"
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		return "tcp_ack_frequency_unavailable"
	}
	// TCP_ACK_FREQUENCY_PARAMETERS contains one UCHAR, not a DWORD.
	parameter := [1]byte{1}
	var returned uint32
	var setErr error
	err = raw.Control(func(fd uintptr) {
		// SIO_TCP_SET_ACK_FREQUENCY = _WSAIOW(IOC_VENDOR, 23).
		setErr = windows.WSAIoctl(windows.Handle(fd), 0x98000017,
			&parameter[0], uint32(len(parameter)), nil, 0, &returned, nil, 0)
	})
	if err != nil || setErr != nil {
		return "tcp_ack_frequency_unavailable"
	}
	return "tcp_ack_every_packet_requested"
}
