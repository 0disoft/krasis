package crashguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const (
	guardHelperEnvironment  = "KRASIS_CRASH_GUARD_HELPER"
	guardMarkerEnvironment  = "KRASIS_CRASH_GUARD_MARKER"
	parentHelperEnvironment = "KRASIS_CRASH_GUARD_PARENT_HELPER"
	hungHelperEnvironment   = "KRASIS_CRASH_GUARD_HUNG_HELPER"
)

func TestCrashGuardReleasesInputAfterParentIsKilled(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "recovered")
	command := exec.Command(os.Args[0], "-test.run=TestCrashGuardParentHelper")
	command.Env = append(os.Environ(), parentHelperEnvironment+"=1", guardMarkerEnvironment+"="+marker)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	ready := bufio.NewReader(stdout)
	line, err := ready.ReadString('\n')
	if err != nil || line != "READY\n" {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("parent readiness = %q, error = %v", line, err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()

	deadline := time.Now().Add(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		payload, readErr := os.ReadFile(marker)
		if readErr == nil {
			if string(payload) != "KEYBOARD_KEY:30" {
				t.Fatalf("recovery marker = %q", payload)
			}
			return
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		if time.Now().After(deadline) {
			t.Fatal("guard did not recover prepared input after parent death")
		}
		<-ticker.C
	}
}

func TestCrashGuardStartRejectsRelativeExecutable(t *testing.T) {
	if _, err := Start("krasis-lab.exe"); err == nil {
		t.Fatal("Start accepted a relative executable path")
	}
}

func TestCrashGuardTimesOutWhenHelperDoesNotAcknowledge(t *testing.T) {
	originalTimeout := processIPCTimeout
	processIPCTimeout = 50 * time.Millisecond
	t.Cleanup(func() { processIPCTimeout = originalTimeout })
	command := exec.Command(os.Args[0], "-test.run=TestCrashGuardHungHelper")
	command.Env = append(os.Environ(), hungHelperEnvironment+"=1")
	process, err := startCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if err := process.PreparePress(token); err == nil || err.Error() != "crash guard IPC timed out" {
		t.Fatalf("PreparePress() error = %v", err)
	}
	select {
	case <-process.Done():
	case <-time.After(time.Second):
		t.Fatal("timed-out crash guard process did not exit")
	}
}

func TestCrashGuardParentHelper(t *testing.T) {
	if os.Getenv(parentHelperEnvironment) != "1" {
		return
	}
	command := exec.Command(os.Args[0], "-test.run=TestCrashGuardProcessHelper")
	command.Env = append(os.Environ(), guardHelperEnvironment+"=1")
	process, err := startCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if err := process.PreparePress(token); err != nil {
		t.Fatal(err)
	}
	fmt.Println("READY")
	<-process.Done()
}

func TestCrashGuardProcessHelper(t *testing.T) {
	if os.Getenv(guardHelperEnvironment) != "1" {
		return
	}
	marker := os.Getenv(guardMarkerEnvironment)
	if marker == "" {
		t.Fatal("missing recovery marker path")
	}
	if err := Serve(os.Stdin, os.Stdout, markerReleasePort{path: marker}); err != nil {
		t.Fatal(err)
	}
}

func TestCrashGuardHungHelper(t *testing.T) {
	if os.Getenv(hungHelperEnvironment) != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

type markerReleasePort struct {
	path string
}

func (port markerReleasePort) Release(_ context.Context, token control.PressedToken) error {
	payload := fmt.Sprintf("%s:%d", token.Kind, token.Code)
	return os.WriteFile(port.path, []byte(payload), 0o600)
}
