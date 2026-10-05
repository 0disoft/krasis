package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestHandoffTagsMatchPeersAndSeparateAttemptsAndReconnect(t *testing.T) {
	fixture := newTLSFixture(t)
	var previous string
	for range 2 {
		serverTLS, clientTLS := openTLSPair(t, mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots), mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName))
		server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
		if serverErr != nil || clientErr != nil {
			t.Fatal("fixture authentication failed")
		}
		tag := server.DiagnosticExchangeTag("private-session", "private-epoch")
		if len(tag) != 64 || tag != client.DiagnosticExchangeTag("private-session", "private-epoch") || tag == previous || tag == server.DiagnosticID() {
			t.Fatal("tag not peer-matched, attempt-scoped or domain-separated")
		}
		if _, err := hex.DecodeString(tag); err != nil {
			t.Fatal("tag not hex")
		}
		if tag == server.DiagnosticExchangeTag("private-session-2", "private-epoch") || tag == server.DiagnosticExchangeTag("private-session", "private-epoch-2") || server.DiagnosticExchangeTag("a", "bc") == server.DiagnosticExchangeTag("ab", "c") {
			t.Fatal("attempt or identifier boundary collision")
		}
		plain := sha256.Sum256([]byte("private-sessionprivate-epoch"))
		if tag == hex.EncodeToString(plain[:]) {
			t.Fatal("tag is guessable from raw identifiers")
		}
		for _, pair := range [][2]string{{"", "e"}, {"s", ""}, {strings.Repeat("s", maxIdentifierSize+1), "e"}} {
			if server.DiagnosticExchangeTag(pair[0], pair[1]) != "" {
				t.Fatal("unbounded or incomplete diagnostic input accepted")
			}
		}
		previous = tag
	}
	if (*PeerConnection)(nil).DiagnosticExchangeTag("s", "e") != "" || (&PeerConnection{}).DiagnosticExchangeTag("s", "e") != "" {
		t.Fatal("unavailable connection invented tag")
	}
}
