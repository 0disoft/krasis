//go:build windows

package agentinstance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestAcquireRejectsSecondOwner(t *testing.T) {
	name := testName(t)
	first, err := acquire(name)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := acquire(name); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire error = %v", err)
	}
}

func TestProcessExitReleasesOwnershipForStaleRecovery(t *testing.T) {
	name := testName(t)
	command := exec.Command(os.Args[0], "-test.run=TestAgentInstanceHelperProcess")
	command.Env = append(os.Environ(), "KRASIS_AGENT_INSTANCE_HELPER=1", "KRASIS_AGENT_INSTANCE_NAME="+name)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("helper error = %v output = %s", err, output)
	}
	lease, err := acquire(name)
	if err != nil {
		t.Fatalf("acquire after helper exit = %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentInstanceHelperProcess(t *testing.T) {
	if os.Getenv("KRASIS_AGENT_INSTANCE_HELPER") != "1" {
		return
	}
	lease, err := acquire(os.Getenv("KRASIS_AGENT_INSTANCE_NAME"))
	if err != nil || lease == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func testName(t *testing.T) string {
	t.Helper()
	value := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	return fmt.Sprintf(`Local\Krasis-Agent-Test-%d-%s`, time.Now().UnixNano(), value)
}
