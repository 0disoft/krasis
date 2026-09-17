//go:build windows

package agentinstance

import (
	"fmt"
	"os"
	"testing"
)

func TestClipboardProbeObservesWithoutAcquiring(t *testing.T) {
	name := fmt.Sprintf("Local\\Krasis-Agent-Test-ClipboardProbe-%d", os.Getpid())
	if exists, err := instanceRunning(name); err != nil || exists {
		t.Fatal(exists, err)
	}
	lease, err := acquire(name)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if exists, err := instanceRunning(name); err != nil || !exists {
		t.Fatal(exists, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if exists, err := instanceRunning(name); err != nil || exists {
		t.Fatal(exists, err)
	}
}
