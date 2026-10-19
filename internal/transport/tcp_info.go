package transport

import "encoding/binary"

// DiagnosticTCPInfo returns only estimated RTT, minimum RTT, retransmitted
// bytes and retransmission-timeout episodes on this socket. It neither sends
// traffic nor changes socket options. Unavailable is never a zero observation.
func (connection *PeerConnection) DiagnosticTCPInfo() ([4]uint32, bool) {
	if connection == nil || connection.connection == nil {
		return [4]uint32{}, false
	}
	return socketTCPInfo(connection.connection.NetConn())
}

// TCP_INFO_v0 has a fixed Windows ABI, including padding after its BOOLEAN.
// Ignore every field except the four numeric diagnostics; never retain socket
// addresses, connection ages, cumulative traffic totals or raw structure bytes.
func decodeTCPInfoV0(data []byte) ([4]uint32, bool) {
	if len(data) != 88 {
		return [4]uint32{}, false
	}
	return [4]uint32{
		binary.LittleEndian.Uint32(data[20:24]),
		binary.LittleEndian.Uint32(data[24:28]),
		binary.LittleEndian.Uint32(data[68:72]),
		binary.LittleEndian.Uint32(data[80:84]),
	}, true
}
