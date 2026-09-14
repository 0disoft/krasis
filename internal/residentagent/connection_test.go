package residentagent

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productcli"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestConnectionCLIThroughAuthenticatedResidentAPI(t *testing.T) {
	now := time.Now().UTC()
	paths, identity, _, err := productidentity.BootstrapWindows(t.TempDir(), now)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := truststore.New(paths.Trust)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trust.Import(transport.AuthenticatedPeerContext{CertificateSHA256: strings.Repeat("a", 64), Role: transport.AuthenticatedPeerListener}, "peer", false, now); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	operations, err := New(ctx, identity.Public(), func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
		<-ctx.Done()
		return context.Cause(ctx)
	}, nil, trust)
	if err != nil {
		t.Fatal(err)
	}
	server, err := localapi.Listen(paths.Control, operations)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if err := server.Close(shutdown); err != nil {
			t.Error(err)
		}
		if err := operations.Close(shutdown); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	for _, test := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"connection", "show", "peer"}, productcli.ExitUnsafeState, "connection_not_configured"},
		{[]string{"connection", "set", "peer", "192.168.1.2:4567", "RIGHT"}, productcli.ExitOK, "connection_saved"},
		{[]string{"connection", "show", "peer", "--json"}, productcli.ExitOK, `"edge":"RIGHT"`},
		{[]string{"connection", "set", "missing", "192.168.1.2:4567", "LEFT"}, productcli.ExitUnsafeState, "peer_not_found"},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"--control-dir", paths.Control}, test.args...)
		code := productcli.Run(ctx, args, &stdout, &stderr, t.TempDir())
		if code != test.code || !strings.Contains(stdout.String(), test.want) || stderr.Len() != 0 {
			t.Fatalf("CLI %v: %d %s %s", test.args, code, stdout.String(), stderr.String())
		}
	}
	client, err := localapi.OpenClient(paths.Control)
	if err != nil {
		t.Fatal(err)
	}
	edgeRequest := localapi.ConnectionRequest{Action: "SET_EDGE", PeerAlias: "peer", Fingerprint: strings.Repeat("a", 64), Edge: "LEFT"}
	edited, err := client.Connection(ctx, edgeRequest)
	if err != nil || edited.Code != "connection_saved" || edited.Profile == nil || edited.Profile.Edge != "LEFT" || edited.Profile.Address != "192.168.1.2:4567" {
		t.Fatalf("edge API: %+v %v", edited, err)
	}
	if _, err := operations.Connect(ctx, localapi.OperationRequest{PeerAlias: "peer"}); err != nil {
		t.Fatal(err)
	}
	request := localapi.ConnectionRequest{SchemaVersion: localapi.ConnectionSchemaVersion, Action: "SET", PeerAlias: "peer", Address: "192.168.1.3:4567", Edge: "LEFT"}
	result, err := operations.Connection(ctx, request)
	if err != nil || result.Code != "active_session_requires_disconnect" {
		t.Fatalf("active update: %+v %v", result, err)
	}
	profile, err := trust.Connection("peer")
	if err != nil || profile.Edge != "LEFT" {
		t.Fatalf("active profile changed: %+v %v", profile, err)
	}
	edgeRequest.Edge = "RIGHT"
	blocked, err := client.Connection(ctx, edgeRequest)
	if err != nil || blocked.Code != "active_session_requires_disconnect" {
		t.Fatalf("active edge write: %+v %v", blocked, err)
	}
}
