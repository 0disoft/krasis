//go:build windows

package crashguard

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const ownershipFixtureEnv = "KRASIS_OWNERSHIP_FIXTURE"

func TestPortableOwnershipFixture(t *testing.T) {
	switch os.Getenv(ownershipFixtureEnv) {
	case "parent":
		owner, err := acquirePortableOwnership(os.Getenv("KRASIS_OWNERSHIP_NAME"))
		if err != nil {
			t.Fatal(err)
		}
		defer owner.Close()
		for _, role := range []string{"input", "guardian"} {
			child := ownershipFixtureCommand(role, owner.argument())
			child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(owner.handle)}}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer child.Process.Kill()
			waitOwnershipCondition(t, func() bool {
				_, err := os.Stat(filepath.Join(os.Getenv("KRASIS_OWNERSHIP_DIR"), role+"-ready"))
				return err == nil
			})
		}
		fmt.Println("READY")
		time.Sleep(10 * time.Second)
	case "input", "guardian":
		owner, err := adoptPortableOwnership(os.Args[len(os.Args)-1])
		if err != nil {
			t.Fatal(err)
		}
		defer owner.Close()
		role := os.Getenv(ownershipFixtureEnv)
		dir := os.Getenv("KRASIS_OWNERSHIP_DIR")
		if err := os.WriteFile(filepath.Join(dir, role+"-ready"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		waitOwnershipCondition(t, func() bool {
			_, err := os.Stat(filepath.Join(dir, role+"-release"))
			return err == nil
		})
		if err := owner.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, role+"-retired"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func ownershipFixtureCommand(role string, args ...string) *exec.Cmd {
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestPortableOwnershipFixture$", "--"}, args...)...)
	command.Env = append(os.Environ(), ownershipFixtureEnv+"="+role)
	configureProcess(command)
	return command
}

func waitOwnershipCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("ownership lifetime did not reach expected state")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func ownershipTestName(t *testing.T) string {
	return fmt.Sprintf(`Local\Krasis-Ownership-Test-%d-%s`, os.Getpid(), t.Name())
}

// No native input is injected. Each hidden child models one independent
// cleanup lifetime; neither parent death nor the first child exit permits
// a new resident to acquire the object.
func TestPortableOwnershipSurvivesResidentAndEitherChild(t *testing.T) {
	for _, first := range []string{"input", "guardian"} {
		t.Run(first, func(t *testing.T) {
			name := ownershipTestName(t)
			dir := t.TempDir()
			t.Setenv("KRASIS_OWNERSHIP_NAME", name)
			t.Setenv("KRASIS_OWNERSHIP_DIR", dir)
			command := ownershipFixtureCommand("parent")
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer command.Wait()
			defer command.Process.Kill()
			defer func() {
				for _, role := range []string{"input", "guardian"} {
					_ = os.WriteFile(filepath.Join(dir, role+"-release"), nil, 0600)
				}
			}()
			ready := make(chan string, 1)
			go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
			select {
			case line := <-ready:
				if line != "READY\n" {
					t.Fatalf("unexpected resident readiness: %q", line)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("resident did not start")
			}
			if err := command.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = command.Wait()
			assertOwnershipDenied(t, name)
			if err := os.WriteFile(filepath.Join(dir, first+"-release"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			waitOwnershipCondition(t, func() bool {
				_, err := os.Stat(filepath.Join(dir, first+"-retired"))
				return err == nil
			})
			// The second child still holds its inherited reference.
			assertOwnershipDenied(t, name)
			second := "input"
			if first == "input" {
				second = "guardian"
			}
			if err := os.WriteFile(filepath.Join(dir, second+"-release"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			waitOwnershipCondition(t, func() bool {
				pending, err := pendingPortableOwnership(name)
				if err != nil {
					t.Fatal(err)
				}
				return !pending
			})
			owner, err := acquirePortableOwnership(name)
			if err != nil {
				t.Fatal("retired pair blocked new resident", err)
			}
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertOwnershipDenied(t *testing.T, name string) {
	t.Helper()
	owner, err := acquirePortableOwnership(name)
	if owner != nil {
		_ = owner.Close()
		t.Fatal("new resident bypassed recovery ownership")
	}
	if !errors.Is(err, ErrExitTimeout) {
		t.Fatal("unexpected ownership admission error", err)
	}
}

func TestPortableOwnershipCloseAndFailedAdmissionDoNotLeak(t *testing.T) {
	name := ownershipTestName(t)
	for range 8 {
		owner, err := acquirePortableOwnership(name)
		if err != nil {
			t.Fatal(err)
		}
		for range 8 {
			assertOwnershipDenied(t, name)
		}
		for range 8 {
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if pending, err := pendingPortableOwnership(name); err != nil || pending {
			t.Fatal("ownership handle leaked", pending, err)
		}
	}
	for _, value := range []string{"", "0", "not-a-handle", "18446744073709551615"} {
		if owner, err := adoptPortableOwnership(value); err == nil || owner != nil {
			t.Fatal("invalid bootstrap accepted", value)
		}
	}
}
