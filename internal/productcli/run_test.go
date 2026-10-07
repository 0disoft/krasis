package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

func TestDiagnosticsExportWorksWithoutAgent(t *testing.T) {
	base := t.TempDir()
	deps := dependencies{localAppData: base, openClient: func(string) (apiClient, error) {
		t.Fatal("offline export opened the agent")
		return nil, nil
	}}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"diagnostics", "export", "--json"}, &stdout, &stderr, deps); code != ExitOK || stderr.Len() != 0 {
		t.Fatalf("export code=%d stderr=%q", code, stderr.String())
	}
	var result diagnosticsExportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "Krasis", "diagnostics", "support-latest.json")
	if result.SchemaVersion != diagnosticsExportSchema || result.Code != "saved" || result.Path != want {
		t.Fatalf("unexpected export result: %+v", result)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"diagnostics", "export", "--control-dir", base}, &stdout, &stderr, deps); code != ExitUsage || stdout.Len() != 0 {
		t.Fatalf("control override code=%d stdout=%q", code, stdout.String())
	}
}

func TestPairRecoveryOutput(t *testing.T) {
	for _, test := range []struct{ code, hint string }{
		{"pairing_completed", "Local trust commit succeeded; remote outcome is unknown"},
		{"pairing_commit_uncertain", "Do not connect or repeat pairing yet"},
		{"pairing_failed", "existing trust may still exist"},
		{"pairing_expired", "did not attempt a local trust commit"},
		{"pairing_cancelled", "remote outcome is unknown"},
		{"confirmation_submitted", "same local job ID"},
		{"confirmation_already_submitted", "same local job ID"},
	} {
		t.Run(test.code, func(t *testing.T) {
			var output bytes.Buffer
			writeHuman(&output, localapi.OperationResult{Status: "FAILED", Code: test.code})
			if !strings.Contains(output.String(), test.hint) {
				t.Fatalf("missing recovery hint: %q", output.String())
			}
		})
	}
}

func TestConnectSaveWarningRetainsSuccessfulRequestAndExplainsPersistence(t *testing.T) {
	value := localapi.OperationResult{Status: "COMPLETED", Code: "connect_started_resume_save_failed"}
	if _, code, err := operation(value, nil); err != nil || code != ExitOK {
		t.Fatal("accepted connection became a failed operation", code, err)
	}
	var output bytes.Buffer
	writeHuman(&output, value)
	if !strings.Contains(output.String(), "automatic reconnect settings were not saved") || !strings.Contains(output.String(), "Do not repeat CONNECT") {
		t.Fatal(output.String())
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
	pairRequest  localapi.OperationRequest
	status       localapi.Status
	operation    localapi.OperationResult
	runtime      localapi.RuntimeInfo
	update       localapi.UpdateResult
	trust        localapi.Trust
	requests     []localapi.UpdateRequest
	updateFn     func(localapi.UpdateRequest) (localapi.UpdateResult, error)
	clipboardFn  func(localapi.ClipboardRequest) (localapi.ClipboardResult, error)
	diagnosticFn func(string) (localapi.DiagnosticTransferResult, error)
	collectionFn func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error)
}

func (client *stubClient) Status(context.Context) (localapi.Status, error) { return client.status, nil }
func (client *stubClient) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	if client.runtime.SchemaVersion == "" {
		return localapi.RuntimeInfo{SchemaVersion: localapi.RuntimeSchemaVersion, Protocol: localapi.AgentProtocol, Generation: localapi.AgentGeneration, Build: "development", Instance: strings.Repeat("a", 32)}, nil
	}
	return client.runtime, nil
}
func (*stubClient) Diagnostics(context.Context) (localapi.Diagnostics, error) {
	return localapi.Diagnostics{SchemaVersion: localapi.DiagnosticsSchemaVersion, State: "ready", Counters: map[string]uint64{}}, nil
}
func (client *stubClient) Trust(context.Context) (localapi.Trust, error) {
	if client.trust.SchemaVersion != "" {
		return client.trust, nil
	}
	return localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion, Peers: []localapi.TrustedPeer{}}, nil
}
func (client *stubClient) Update(_ context.Context, request localapi.UpdateRequest) (localapi.UpdateResult, error) {
	client.requests = append(client.requests, request)
	if client.updateFn != nil {
		return client.updateFn(request)
	}
	return client.update, nil
}
func (client *stubClient) SendDiagnostic(_ context.Context, peer string) (localapi.DiagnosticTransferResult, error) {
	if client.diagnosticFn != nil {
		return client.diagnosticFn(peer)
	}
	return localapi.DiagnosticTransferResult{Code: "rejected"}, nil
}
func (client *stubClient) CollectDiagnostic(_ context.Context, request localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
	if client.collectionFn != nil {
		return client.collectionFn(request)
	}
	return localapi.DiagnosticCollectionResult{Code: "rejected"}, nil
}
func (client *stubClient) Clipboard(_ context.Context, request localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
	if client.clipboardFn != nil {
		return client.clipboardFn(request)
	}
	return localapi.ClipboardResult{Code: "rejected"}, nil
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
