package transport

// ACKPolicy reports only whether the socket request was accepted. It is not an
// input acknowledgement, worker receipt, peer heartbeat or performance result.
func (connection *PeerConnection) ACKPolicy() string {
	if connection == nil || connection.ackPolicy == "" {
		return "tcp_ack_frequency_unavailable"
	}
	return connection.ackPolicy
}
