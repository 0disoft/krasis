//go:build windows

package crashguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"golang.org/x/sys/windows"
)

const portableGuardianFixture = "KRASIS_PORTABLE_GUARDIAN_FIXTURE"

func TestPortableGuardianFixture(t *testing.T) {
	mode := os.Getenv(portableGuardianFixture)
	if mode == "" {
		return
	}
	switch mode {
	case "input", "input-cooperative":
		// Deliberately ignores cancellation/EOF; only native retirement stops
		// it. This fixture performs no keyboard/mouse injection.
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if line != "DOWN\n" {
			t.Fatal("missing simulated input")
		}
		fmt.Println("APPLIED")
		if mode == "input-cooperative" {
			command, err := readCommand(os.Stdin)
			if err != nil || command.operation != operationDisarm {
				t.Fatal("missing child disarm", err)
			}
			if err := writeAck(os.Stdout, operationDisarm, statusOK); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Second)
	case "guardian":
		parent, err := strconv.ParseUint(os.Args[len(os.Args)-2], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		input, err := strconv.ParseUint(os.Args[len(os.Args)-1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		release := retiredMarkerRelease{input: windows.Handle(input), path: os.Getenv(guardMarkerEnvironment)}
		result, err := ServePortableGuardian(uintptr(parent), uintptr(input), os.Stdin, os.Stdout, release)
		if result.Released != 1 || result.Remaining != 0 || err != nil {
			t.Fatal(result, err)
		}
	case "parent":
		input, err := startCommand(guardianFixtureCommand("input"))
		if err != nil {
			t.Fatal(err)
		}
		input.portable = true
		defer input.process.Kill()
		guard, err := startPortableGuardian(guardianFixtureCommand("guardian"), input)
		if err != nil {
			t.Fatal(err)
		}
		defer guard.Close()
		if _, err := guard.PreparePress(control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); err != nil {
			t.Fatal(err)
		}
		acknowledgeFixtureDown(t, input)
		fmt.Println("READY")
		<-guard.Done()
	}
}

func acknowledgeFixtureDown(t *testing.T, input *Process) {
	t.Helper()
	if _, err := input.stdin.Write([]byte("DOWN\n")); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(input.stdout).ReadString('\n'); err != nil || line != "APPLIED\n" {
		t.Fatal("missing simulated down acknowledgement", line, err)
	}
}

func guardianFixtureCommand(mode string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestPortableGuardianFixture$", "--")
	command.Env = append(os.Environ(), portableGuardianFixture+"="+mode)
	return command
}

type retiredMarkerRelease struct {
	input windows.Handle
	path  string
}

func (release retiredMarkerRelease) Release(ctx context.Context, token control.PressedToken) error {
	state, err := windows.WaitForSingleObject(release.input, 0)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		return errors.New("release preceded exact input child exit")
	}
	return (markerReleasePort{path: release.path}).Release(ctx, token)
}

func TestPortableGuardianStopsUncooperativeInputAfterParentDeath(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "recovered")
	t.Setenv(guardMarkerEnvironment, marker)
	command := guardianFixtureCommand("parent")
	configureProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()
	defer command.Process.Kill()
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "READY\n" {
			t.Fatalf("parent/guardian readiness: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent/guardian failed to acknowledge journal")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(marker); err == nil {
			if string(data) != "KEYBOARD_KEY:30" {
				t.Fatal("wrong recovery marker", string(data))
			}
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("guardian failed to retire input and recover after parent death")
		}
	}
}

func TestGuardianPinnedWaitNeverRetriesFailedHandle(t *testing.T) {
	for _, state := range []uint32{windows.WAIT_FAILED, windows.WAIT_ABANDONED} {
		calls := 0
		err := waitPinnedGuardianProcess(context.Background(), windows.Handle(123), func(handle windows.Handle, _ uint32) (uint32, error) {
			calls++
			return state, nil
		})
		if err == nil || calls != 1 {
			t.Fatal("failed handle value was reused", err, calls)
		}
	}
	calls := 0
	err := waitPinnedGuardianProcess(context.Background(), windows.Handle(123), func(windows.Handle, uint32) (uint32, error) {
		calls++
		if calls < 3 {
			return uint32(windows.WAIT_TIMEOUT), nil
		}
		return windows.WAIT_OBJECT_0, nil
	})
	if err != nil || calls != 3 {
		t.Fatal("timeout could not continue the pinned wait", err, calls)
	}
	calls = 0
	err = waitPinnedGuardianProcess(context.Background(), windows.Handle(123), func(windows.Handle, uint32) (uint32, error) {
		calls++
		return windows.WAIT_FAILED, windows.ERROR_INVALID_HANDLE
	})
	if !errors.Is(err, windows.ERROR_INVALID_HANDLE) || calls != 1 {
		t.Fatal("native wait error retried a poisoned handle", err, calls)
	}
}

func TestPortableGuardianDisarmJoinsInputAndRepeatedClose(t *testing.T) {
	t.Setenv(guardMarkerEnvironment, filepath.Join(t.TempDir(), "recovered"))
	input, err := startCommand(guardianFixtureCommand("input-cooperative"))
	if err != nil {
		t.Fatal(err)
	}
	input.portable = true
	defer func() { <-input.requestAbort(); _ = input.Close() }()
	guard, err := startPortableGuardian(guardianFixtureCommand("guardian"), input)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	processes := &PortableProcesses{input: input, guardian: guard}
	if _, err := guard.PreparePress(control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); err != nil {
		t.Fatal(err)
	}
	acknowledgeFixtureDown(t, input)
	for range 8 {
		if err := processes.Close(); err != nil {
			t.Fatal("disarm did not join the exact input lifetime and guardian monitor", err)
		}
	}
	select {
	case <-input.Done():
	case <-time.After(time.Second):
		t.Fatal("guardian retired before the input child")
	}
	if data, err := os.ReadFile(os.Getenv(guardMarkerEnvironment)); err != nil || string(data) != "KEYBOARD_KEY:30" {
		t.Fatal("normal disarm lost held-input recovery", string(data), err)
	}
}

func TestGuardianDeniedTerminationStillRequiresExactExit(t *testing.T) {
	input, err := startCommand(guardianFixtureCommand("input"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { <-input.requestAbort(); _ = input.Close() }()
	acknowledgeFixtureDown(t, input)
	var pinned windows.Handle
	var duplicateErr error
	err = input.process.WithHandle(func(handle uintptr) {
		duplicateErr = windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(handle), windows.CurrentProcess(), &pinned, windows.SYNCHRONIZE, false, 0)
	})
	if err = errors.Join(err, duplicateErr); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(pinned)
	retirement := &guardianRetirement{input: pinned}
	done := make(chan error, 1)
	go func() { done <- retirement.retire() }()
	select {
	case err := <-done:
		t.Fatal("denied termination was treated as process exit", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := input.process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("exact exit proof did not complete retirement", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pinned input exit was not observed")
	}
}
