package residentagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestConnectSessionOutlivesRequestAndDisconnectStopsIt(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	runner := func(ctx context.Context, alias string) error {
		if alias != "peer" {
			t.Errorf("alias = %q", alias)
		}
		close(started)
		<-ctx.Done()
		close(stopped)
		return context.Cause(ctx)
	}
	operations := testOperations(t, runner)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	result, err := operations.Connect(requestCtx, localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatalf("Connect() result = %+v error = %v", result, err)
	}
	<-started
	cancelRequest()
	select {
	case <-stopped:
		t.Fatal("request cancellation stopped the resident session")
	case <-time.After(20 * time.Millisecond):
	}
	result, err = operations.Disconnect(context.Background(), localapi.OperationRequest{})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatalf("Disconnect() result = %+v error = %v", result, err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not stop the resident session")
	}
}

func TestSafeStopCancelsAgentRoot(t *testing.T) {
	operations := testOperations(t, nil)
	result, err := operations.SafeStop(context.Background())
	if err != nil || result.Code != "safe_stop_requested" {
		t.Fatalf("SafeStop() result = %+v error = %v", result, err)
	}
	select {
	case <-operations.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("safe stop did not cancel agent root")
	}
}

func TestStatusAndDiagnosticsAreRedactedAggregates(t *testing.T) {
	operations := testOperations(t, nil)
	status, err := operations.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.DeviceFingerprint != strings.Repeat("a", 64) || status.IdentityGeneration != 3 || status.State != "LOCAL" {
		t.Fatalf("status = %+v", status)
	}
	result, err := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Code != "not_configured" {
		t.Fatalf("Connect() result = %+v error = %v", result, err)
	}
	diagnostics, err := operations.Diagnostics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Counters["not_configured"] != 1 || diagnostics.State != "not_configured" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func testOperations(t *testing.T, runner SessionRunner) *Operations {
	t.Helper()
	operations, err := New(context.Background(), productidentity.PublicIdentity{
		SchemaVersion: productidentity.SchemaVersion,
		Fingerprint:   strings.Repeat("a", 64),
		Generation:    3,
		CreatedAt:     time.Now().UTC(),
	}, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	return operations
}
