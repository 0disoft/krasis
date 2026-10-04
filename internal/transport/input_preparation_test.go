package transport

import (
	"errors"
	"testing"
)

func TestPreparationALPNRequiresBothProductEndpoints(t *testing.T) {
	for _, opt := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		fixture := newTLSFixture(t)
		serverConfig := mustServerTLSConfig(t, fixture.serverIdentity, fixture.roots)
		clientConfig := mustClientTLSConfig(t, fixture.clientIdentity, fixture.roots, fixture.serverName)
		if opt[0] {
			EnableInputPreparation(serverConfig)
		}
		if opt[1] {
			EnableInputPreparation(clientConfig)
		}
		serverTLS, clientTLS := openTLSPair(t, serverConfig, clientConfig)
		server, client, serverErr, clientErr := authenticateConnectionPair(t, serverTLS, clientTLS)
		if serverErr != nil || clientErr != nil {
			t.Fatalf("mixed peer authentication failed: %v %v", serverErr, clientErr)
		}
		want := opt[0] && opt[1]
		if server.SupportsInputPreparation() != want || client.SupportsInputPreparation() != want {
			t.Fatal("capability enabled without both product endpoints")
		}
	}
}

func TestPreparationPayloadHasNoAuthorityAndCannotDowngrade(t *testing.T) {
	command := ControlCommand{Kind: ControlPrepareInput}
	data, err := EncodeControlPayload(command)
	if err != nil || len(data) != controlHeaderSize || data[0] != 4 {
		t.Fatal("preparation envelope invalid")
	}
	if decoded, err := DecodeControlPayload(data); err != nil || decoded != command {
		t.Fatal("preparation round trip failed")
	}
	data[0] = ControlPayloadVersion
	if _, err := DecodeControlPayload(data); !errors.Is(err, ErrUnsupportedControlPayload) {
		t.Fatal("preparation downgrade accepted")
	}
	for _, mutate := range []func(*ControlCommand){
		func(c *ControlCommand) { c.SessionID = "s" },
		func(c *ControlCommand) { c.LeaseID = "l" },
		func(c *ControlCommand) { c.Epoch = "e" },
		func(c *ControlCommand) { c.Edge = "RIGHT" },
	} {
		c := command
		mutate(&c)
		if _, err := EncodeControlPayload(c); err == nil {
			t.Fatal("preparation carries authority")
		}
	}
	request, err := EncodeControlPayload(ControlCommand{Kind: ControlLeaseRequest, SessionID: "s", Epoch: "e", Edge: "RIGHT"})
	if err != nil || request[0] != ControlPayloadVersion {
		t.Fatal("legacy lease encoding changed")
	}
	request[0] = 4
	if _, err := DecodeControlPayload(request); !errors.Is(err, ErrUnsupportedControlPayload) {
		t.Fatal("v4 used to introduce a lease")
	}
}
