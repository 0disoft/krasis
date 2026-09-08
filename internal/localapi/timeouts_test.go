package localapi

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestClientReceivesShutdownTimeoutResultAfterFullStopBudget(t *testing.T) {
	control := filepath.Join(protectedRoot(t), "control")
	server, err := Listen(control, &slowDisconnectOperations{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("server shutdown: %v", err)
		}
	})
	client, err := OpenClient(control)
	if err != nil {
		t.Fatal(err)
	}
	if server.http.WriteTimeout <= server.http.ReadTimeout+DefaultSessionStopTimeout {
		t.Fatal("server response deadline leaves no headroom after reading and stopping")
	}
	if client.http.Timeout <= server.http.WriteTimeout {
		t.Fatal("client deadline expires before the server response budget")
	}
	result, err := client.Disconnect(context.Background())
	if err != nil || result.Status != "FAILED" || result.Code != "shutdown_timeout" {
		t.Fatalf("disconnect must preserve the operation result: %+v, %v", result, err)
	}
}

type slowDisconnectOperations struct{ stubOperations }

func (*slowDisconnectOperations) Disconnect(ctx context.Context, _ OperationRequest) (OperationResult, error) {
	timer := time.NewTimer(DefaultSessionStopTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return OperationResult{SchemaVersion: ResultSchemaVersion, Status: "FAILED", Code: "shutdown_timeout"}, nil
	case <-ctx.Done():
		return OperationResult{}, ctx.Err()
	}
}
