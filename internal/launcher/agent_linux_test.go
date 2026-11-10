//go:build linux

package launcher

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productcli"
)

func TestLinuxCLIStartsExplicitlyWithoutChangingReadOnlyBehavior(t *testing.T) {
	for _, args := range [][]string{{"--json", "start"}, {"agent", "start"}, {"listen"}} {
		var output, failure bytes.Buffer
		starts := 0
		code := runProductCLI(context.Background(), args, &output, &failure, t.TempDir(), func(context.Context, string, string) error { starts++; return nil })
		if code != productcli.ExitOK || starts != 1 || failure.Len() != 0 {
			t.Fatalf("start %v = %d, starts=%d, stderr=%s", args, code, starts, failure.String())
		}
		if args[0] == "--json" && !strings.Contains(output.String(), `"code":"agent_ready"`) {
			t.Fatalf("start JSON = %s", output.String())
		}
	}
	for _, args := range [][]string{{"help"}, {"version"}, {"status"}, {"doctor"}, {"connect"}, {"safe-stop"}, {"--json", "--json", "start"}, {"--control-dir", "/custom", "connect", "peer"}} {
		var output, failure bytes.Buffer
		_ = runProductCLI(context.Background(), args, &output, &failure, t.TempDir(), func(context.Context, string, string) error {
			t.Fatalf("%v unexpectedly started agent", args)
			return nil
		})
	}
}

func TestLinuxCLIAutostartFailureDoesNotLeakPaths(t *testing.T) {
	var output, failure bytes.Buffer
	starts := 0
	code := runProductCLI(context.Background(), []string{"connect", "peer"}, &output, &failure, t.TempDir(), func(context.Context, string, string) error {
		starts++
		return errors.New("private agent path and environment")
	})
	if code != productcli.ExitAgentUnavailable || starts != 1 || strings.Contains(failure.String(), "private") {
		t.Fatalf("autostart failure = %d, starts=%d, stderr=%s", code, starts, failure.String())
	}
}

func TestLinuxAgentLaunchPinsImageAndPreservesDesktopEnvironment(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "environment")
	agent := filepath.Join(base, "krasis-agent")
	script := "#!/bin/sh\nprintf '%s\\n' \"$DISPLAY\" \"$XAUTHORITY\" \"$XDG_DATA_HOME\" > \"$KRASIS_LINUX_TEST_RESULT\"\n"
	if err := os.WriteFile(agent, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISPLAY", ":test-display")
	t.Setenv("XAUTHORITY", "/test/xauthority")
	t.Setenv("XDG_DATA_HOME", "/old-data-home")
	t.Setenv("KRASIS_LINUX_TEST_RESULT", result)
	dataHome := filepath.Join(base, "data")
	if err := startSiblingAgent(filepath.Join(base, "krasis-cli"), dataHome); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(result)
		if err == nil && len(data) > 0 {
			if string(data) != ":test-display\n/test/xauthority\n"+dataHome+"\n" {
				t.Fatalf("agent environment = %q", data)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent environment result = %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.Chmod(agent, 0o722); err != nil {
		t.Fatal(err)
	}
	if err := startSiblingAgent(filepath.Join(base, "krasis-cli"), dataHome); err == nil {
		t.Fatal("group/world-writable executable accepted")
	}
	if err := os.Remove(agent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/sh", agent); err != nil {
		t.Fatal(err)
	}
	if err := startSiblingAgent(filepath.Join(base, "krasis-cli"), dataHome); err == nil {
		t.Fatal("symlink executable accepted")
	}
}

func TestLinuxEnsureAgentHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := EnsureAgent(ctx, "/unused/data", "/unused/krasis-cli"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
}
