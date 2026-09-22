package transport

import "testing"

func TestDiagnosticCorrelationMatchesPeersButChangesOnReconnect(t *testing.T) {
	fixture := newTLSFixture(t)
	var previous string
	for range 2 {
		serverTLS, clientTLS := openTLSPair(t, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots), mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
		server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
		if serverErr != nil || clientErr != nil {
			t.Fatalf("authentication: %v / %v", serverErr, clientErr)
		}
		id := server.DiagnosticID()
		if len(id) != 64 || id != client.DiagnosticID() || id == previous {
			t.Fatalf("invalid correlation: %q / %q", id, client.DiagnosticID())
		}
		previous = id
	}
}
