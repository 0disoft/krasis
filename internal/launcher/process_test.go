package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerRequestStopCreatesExclusiveSignal(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".stop")
	manager := &Manager{running: true, stopFile: path}
	if err := manager.RequestStop(); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(path); err != nil || string(payload) != "stop\n" {
		t.Fatalf("stop signal = %q, %v", payload, err)
	}
	if err := manager.RequestStop(); err != nil {
		t.Fatalf("repeated RequestStop = %v", err)
	}
}

func TestManagerRequestStopRejectsUnsafeOperation(t *testing.T) {
	manager := &Manager{running: true}
	if err := manager.RequestStop(); err == nil {
		t.Fatal("RequestStop accepted an operation without a safe stop file")
	}
	manager.running = false
	if err := manager.RequestStop(); err == nil || errors.Is(err, os.ErrExist) {
		t.Fatalf("RequestStop without a running operation = %v", err)
	}
}
