package residentagent

func (connection *disconnectConnection) DiagnosticReceiveQueue() (uint32, bool) {
	if connection != nil {
		if source, ok := connection.Connection.(interface{ DiagnosticReceiveQueue() (uint32, bool) }); ok {
			return source.DiagnosticReceiveQueue()
		}
	}
	return 0, false
}
