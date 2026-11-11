//go:build linux

package agentinstance

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestWatchLeaseCoexistsWithAgentAndRejectsSecondWatcher(t *testing.T) {
	dir, err := unix.Open(t.TempDir(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dir)
	agent, err := acquireAt(dir, productionName)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	first, err := acquireAt(dir, watchName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireAt(dir, watchName); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second watcher = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireAt(dir, watchName)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
