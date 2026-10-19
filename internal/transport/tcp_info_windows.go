//go:build windows

package transport

import (
	"net"

	"golang.org/x/sys/windows"
)

func socketTCPInfo(connection net.Conn) ([4]uint32, bool) {
	tcp, ok := connection.(*net.TCPConn)
	if !ok {
		return [4]uint32{}, false
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		return [4]uint32{}, false
	}
	var version [4]byte // DWORD version 0
	var data [88]byte
	var returned uint32
	var queryErr error
	err = raw.Control(func(fd uintptr) {
		// _WSAIORW(IOC_VENDOR, 39), supported since Windows 10 1703.
		// Synchronous read-only query; no pending I/O, retry or socket mutation.
		queryErr = windows.WSAIoctl(windows.Handle(fd), 0xd8000027,
			&version[0], uint32(len(version)), &data[0], uint32(len(data)), &returned, nil, 0)
	})
	if err != nil || queryErr != nil || returned != uint32(len(data)) {
		return [4]uint32{}, false
	}
	return decodeTCPInfoV0(data[:])
}
