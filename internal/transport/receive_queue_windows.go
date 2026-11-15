//go:build windows

package transport

import (
	"encoding/binary"
	"golang.org/x/sys/windows"
	"net"
)

func socketReceiveQueue(connection net.Conn) (uint32, bool) {
	tcp, ok := connection.(*net.TCPConn)
	if !ok {
		return 0, false
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		return 0, false
	}
	var data [4]byte
	var returned uint32
	var queryErr error
	err = raw.Control(func(fd uintptr) {
		// FIONREAD: read-only, no event registration or socket option changes.
		queryErr = windows.WSAIoctl(windows.Handle(fd), 0x4004667f,
			nil, 0, &data[0], 4, &returned, nil, 0)
	})
	if err != nil || queryErr != nil || returned != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(data[:]), true
}
