package residentagent

// Keep the optional live-socket query through the product disconnect wrapper.
// The supervisor binds it to the new connection's bounded handoff collector.
func (connection *disconnectConnection) DiagnosticTCPInfo() ([4]uint32, bool) {
	if connection != nil {
		if query, ok := connection.Connection.(interface{ DiagnosticTCPInfo() ([4]uint32, bool) }); ok {
			return query.DiagnosticTCPInfo()
		}
	}
	return [4]uint32{}, false
}
