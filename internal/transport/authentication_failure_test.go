package transport

import (
	"context"
	"testing"
	"time"
)

func TestAuthenticationFailureClassesUseActualTLSChecks(t *testing.T) {
	fixture := newTLSFixture(t)
	for _, test := range []struct{ name, want string }{{"identity", "tls_identity_mismatch"}, {"time", "tls_certificate_time_invalid"}, {"alpn", "tls_alpn_rejected"}} {
		t.Run(test.name, func(t *testing.T) {
			serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
			clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)
			switch test.name {
			case "identity":
				clientConfig.ServerName = "different.invalid"
			case "time":
				clientConfig.Time = func() time.Time { return time.Now().Add(100 * 365 * 24 * time.Hour) }
			case "alpn":
				serverConfig.NextProtos = []string{"other/1"}
				clientConfig.NextProtos = []string{"other/1"}
			}
			_, _, _, err := handshakePair(t, serverConfig, clientConfig)
			if err == nil || AuthenticationFailureCode(err) != test.want {
				t.Fatalf("classification: %s error=%v", AuthenticationFailureCode(err), err)
			}
		})
	}
	server, _ := openTLSPair(t, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots), mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := AuthenticatePeerContext(ctx, server); AuthenticationFailureCode(err) != "tls_timeout" {
		t.Fatalf("handshake deadline: %v", err)
	}
}
