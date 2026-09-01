package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServerAuthenticatesAndDispatchesBoundedOperations(t *testing.T) {
	control := filepath.Join(protectedRoot(t), "control")
	operations := &stubOperations{}
	server, err := Listen(control, operations)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	credentials, err := LoadClientCredentials(control)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	unauthorized, err := client.Get("http://" + credentials.Endpoint.Address + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}
	_ = unauthorized.Body.Close()

	status := request(t, client, credentials, http.MethodGet, "/v1/status", "")
	if status.StatusCode != http.StatusOK || !strings.Contains(readBody(t, status), `"state":"LOCAL"`) {
		t.Fatalf("status = %d", status.StatusCode)
	}
	body := `{"schemaVersion":"krasis.agent.operation/v1","peerAlias":"peer"}`
	connected := request(t, client, credentials, http.MethodPost, "/v1/connect", body)
	if connected.StatusCode != http.StatusOK || !strings.Contains(readBody(t, connected), `"code":"completed"`) {
		t.Fatalf("connect status = %d", connected.StatusCode)
	}
	operations.mu.Lock()
	connectCalls := operations.connectCalls
	operations.mu.Unlock()
	if connectCalls != 1 {
		t.Fatalf("connect calls = %d", connectCalls)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServerRejectsUnknownTrailingAndOversizedRequestsBeforeDispatch(t *testing.T) {
	control := filepath.Join(protectedRoot(t), "control")
	operations := &stubOperations{}
	server, err := Listen(control, operations)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	credentials, err := LoadClientCredentials(control)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	for _, body := range []string{
		`{"schemaVersion":"krasis.agent.operation/v1","unknown":true}`,
		`{"schemaVersion":"krasis.agent.operation/v1"} {}`,
		`{"schemaVersion":"krasis.agent.operation/v1","peerAlias":"peer\nforged"}`,
		`{"schemaVersion":"krasis.agent.operation/v1","peerAlias":"` + strings.Repeat("x", maxRequestBytes) + `"}`,
	} {
		response := request(t, client, credentials, http.MethodPost, "/v1/connect", body)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d for body length %d", response.StatusCode, len(body))
		}
		_ = response.Body.Close()
	}
	operations.mu.Lock()
	calls := operations.connectCalls
	operations.mu.Unlock()
	if calls != 0 {
		t.Fatalf("connect calls = %d", calls)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type stubOperations struct {
	mu           sync.Mutex
	connectCalls int
}

func (*stubOperations) Status(context.Context) (Status, error) {
	return Status{SchemaVersion: StatusSchemaVersion, State: "LOCAL", Actions: []string{}}, nil
}
func (operations *stubOperations) Connect(context.Context, OperationRequest) (OperationResult, error) {
	operations.mu.Lock()
	operations.connectCalls++
	operations.mu.Unlock()
	return completedResult(), nil
}
func (*stubOperations) Disconnect(context.Context, OperationRequest) (OperationResult, error) {
	return completedResult(), nil
}
func (*stubOperations) Pair(context.Context, OperationRequest) (OperationResult, error) {
	return completedResult(), nil
}
func (*stubOperations) Diagnostics(context.Context) (Diagnostics, error) {
	return Diagnostics{SchemaVersion: DiagnosticsSchemaVersion, State: "LOCAL", Counters: map[string]uint64{}}, nil
}
func (*stubOperations) Trust(context.Context) (Trust, error) {
	return Trust{SchemaVersion: TrustSchemaVersion, Peers: []TrustedPeer{}}, nil
}
func (*stubOperations) RevokeTrust(context.Context, OperationRequest) (OperationResult, error) {
	return completedResult(), nil
}
func (*stubOperations) Topology(context.Context) (Topology, error) {
	return Topology{SchemaVersion: TopologySchemaVersion, State: "LOCAL", Layout: "UNCONFIGURED", Peers: []TopologyPeer{}}, nil
}
func (*stubOperations) SafeStop(context.Context) (OperationResult, error) {
	return completedResult(), nil
}

func completedResult() OperationResult {
	return OperationResult{SchemaVersion: ResultSchemaVersion, Status: "COMPLETED", Code: "completed"}
}

func request(t *testing.T, client *http.Client, credentials ClientCredentials, method, path, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, "http://"+credentials.Endpoint.Address+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+credentials.Token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("response is not JSON: %s", data)
	}
	return string(data)
}
