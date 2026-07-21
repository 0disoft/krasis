package labcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorAgentStopFileCancelsAndConsumesSignal(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".stop")
	ctx, closeMonitor, err := monitorAgentStopFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMonitor()
	if err := os.WriteFile(path, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), errAgentStopRequested) {
			t.Fatalf("context cause = %v", context.Cause(ctx))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop file did not cancel the agent context")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stop signal was not consumed: %v", err)
	}
}

func TestMonitorAgentStopFileRejectsExistingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".stop")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := monitorAgentStopFile(context.Background(), path); err == nil {
		t.Fatal("monitorAgentStopFile accepted an existing signal")
	}
}
