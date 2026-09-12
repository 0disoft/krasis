package productcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestRunProducesStableJSONAndHumanOutput(t *testing.T) {
	client := &stubClient{status: localapi.Status{
		SchemaVersion: localapi.StatusSchemaVersion, State: "LOCAL", Actions: []string{"PAIR"},
		DeviceFingerprint: strings.Repeat("a", 64), IdentityGeneration: 1,
	}}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"status", "--json"}, want: `"schemaVersion":"krasis.agent.status/v1"`},
		{args: []string{"status"}, want: "State: LOCAL"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), test.args, &stdout, &stderr, testDependencies(client))
		if code != ExitOK || !strings.Contains(stdout.String(), test.want) || stderr.Len() != 0 {
			t.Fatalf("run(%v) code=%d stdout=%q stderr=%q", test.args, code, stdout.String(), stderr.String())
		}
	}
}

func TestRunMapsUnavailableAndOperationOutcomes(t *testing.T) {
	tests := []struct {
		name string
		deps dependencies
		args []string
		want int
	}{
		{name: "unavailable", deps: dependencies{localAppData: `C:\Users\test\AppData\Local`, openClient: func(string) (apiClient, error) { return nil, localapi.ErrUnavailable }}, args: []string{"status"}, want: ExitAgentUnavailable},
		{name: "policy", deps: testDependencies(&stubClient{operation: localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: "REJECTED", Code: "not_configured"}}), args: []string{"connect", "peer"}, want: ExitPolicyDenied},
		{name: "unsafe", deps: testDependencies(&stubClient{operation: localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: "REJECTED", Code: "already_active"}}), args: []string{"connect", "peer"}, want: ExitUnsafeState},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), test.args, &stdout, &stderr, test.deps); code != test.want {
				t.Fatalf("code = %d, want %d; stderr=%q", code, test.want, stderr.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	secretPath := `C:\Users\secret-user\AppData\Local\Krasis\control`
	deps := dependencies{localAppData: `C:\Users\test\AppData\Local`, openClient: func(string) (apiClient, error) { return nil, localapi.ErrUnavailable }}
	if code := run(context.Background(), []string{"status", "--control-dir", secretPath}, &stdout, &stderr, deps); code != ExitAgentUnavailable || strings.Contains(stderr.String(), secretPath) || strings.Contains(stderr.String(), "secret-user") {
		t.Fatalf("redacted failure code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunRejectsUnknownAndMalformedCommandsBeforeOpeningAgent(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"status", "extra"}, {"--unknown", "status"}, {"pair", "begin", "peer"}} {
		opened := false
		deps := dependencies{localAppData: `C:\Users\test\AppData\Local`, openClient: func(string) (apiClient, error) {
			opened = true
			return nil, errors.New("unexpected")
		}}
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr, deps); code != ExitUsage || opened {
			t.Fatalf("run(%v) code=%d opened=%t", args, code, opened)
		}
	}
}

func testDependencies(client apiClient) dependencies {
	return dependencies{
		localAppData: `C:\Users\test\AppData\Local`,
		openClient:   func(string) (apiClient, error) { return client, nil },
	}
}

type stubClient struct {
	pairRequest localapi.OperationRequest
	status      localapi.Status
	operation   localapi.OperationResult
}

func (client *stubClient) Status(context.Context) (localapi.Status, error) { return client.status, nil }
func (*stubClient) Diagnostics(context.Context) (localapi.Diagnostics, error) {
	return localapi.Diagnostics{SchemaVersion: localapi.DiagnosticsSchemaVersion, State: "ready", Counters: map[string]uint64{}}, nil
}
func (*stubClient) Trust(context.Context) (localapi.Trust, error) {
	return localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion, Peers: []localapi.TrustedPeer{}}, nil
}
func (*stubClient) Topology(context.Context) (localapi.Topology, error) {
	return localapi.Topology{SchemaVersion: localapi.TopologySchemaVersion, State: "LOCAL", Layout: "UNCONFIGURED", Peers: []localapi.TopologyPeer{}}, nil
}
func (client *stubClient) Connect(context.Context, string) (localapi.OperationResult, error) {
	return client.operation, nil
}
func (client *stubClient) Disconnect(context.Context) (localapi.OperationResult, error) {
	return client.operation, nil
}
func (client *stubClient) Pair(_ context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	client.pairRequest = request
	return client.operation, nil
}

func TestPairInboundOptionsAreExplicitAndExclusive(t *testing.T) {
	for _, action := range []string{"listen", "dial"} {
		for _, option := range []string{"--allow-inbound-control", "--deny-inbound-control"} {
			for _, replace := range []bool{false, true} {
				client := &stubClient{operation: localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: "COMPLETED", Code: "pairing_waiting"}}
				args := []string{"pair", action, "peer", "127.0.0.1:12345", option}
				if replace {
					args = append(args, "--replace", strings.Repeat("a", 64))
				}
				var out bytes.Buffer
				if code := run(context.Background(), args, &out, &out, testDependencies(client)); code != ExitOK {
					t.Fatalf("%v: exit %d", args, code)
				}
				request := client.pairRequest
				if request.PairingAction != strings.ToUpper(action) || request.AllowInboundControl == nil || *request.AllowInboundControl != (option == "--allow-inbound-control") || (request.Fingerprint != "") != replace {
					t.Fatalf("wrong request: %+v", request)
				}
			}
		}
	}
	for _, tail := range [][]string{nil, {"--deny-inbound-control", "--allow-inbound-control"}, {"--allow-inbound-control", "--deny-inbound-control"}, {"--deny-inbound-control", "--deny-inbound-control"}, {"--allow-inbound-control", "--allow-inbound-control"}, {"--deny-inbound-control", "--replace", strings.Repeat("a", 64), "--allow-inbound-control"}} {
		opened := false
		deps := dependencies{localAppData: `C:\Users\test\AppData\Local`, openClient: func(string) (apiClient, error) { opened = true; return nil, errors.New("unexpected") }}
		var out bytes.Buffer
		args := append([]string{"pair", "listen", "peer", "127.0.0.1:12345"}, tail...)
		if code := run(context.Background(), args, &out, &out, deps); code != ExitUsage || opened {
			t.Fatalf("ambiguous options %v: exit=%d opened=%t", args, code, opened)
		}
	}
}
func (client *stubClient) RevokeTrust(context.Context, string) (localapi.OperationResult, error) {
	return client.operation, nil
}
func (client *stubClient) SafeStop(context.Context) (localapi.OperationResult, error) {
	return client.operation, nil
}
