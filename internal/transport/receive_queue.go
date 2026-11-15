package transport

// DiagnosticReceiveQueue counts unread encrypted socket bytes without consuming
// them. TLS buffers and pending overlapped receive buffers are not included.
func (connection *PeerConnection) DiagnosticReceiveQueue() (uint32, bool) {
	if connection == nil || connection.connection == nil {
		return 0, false
	}
	return socketReceiveQueue(connection.connection.NetConn())
}
