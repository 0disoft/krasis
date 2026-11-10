//go:build linux

package launchrecord

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLinuxIndependentProcessDetachesAndReaps(t *testing.T) {
	result := filepath.Join(t.TempDir(), "result.json")
	command := exec.Command(os.Args[0], "-test.run=^TestLinuxIndependentChild$")
	command.Dir = filepath.Dir(os.Args[0])
	command.Env = append(os.Environ(), "KRASIS_LINUX_PROCESS_TEST="+result)
	if err := StartIndependent(command); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(command.Process.Pid)))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		select {
		case <-ctx.Done():
			_ = command.Process.Kill()
			t.Fatal("independent child was not reaped")
		case <-time.After(10 * time.Millisecond):
		}
	}
	data, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var value struct{ PID, Session int }
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value.PID != command.Process.Pid || value.Session != value.PID {
		t.Fatalf("child did not create a new session: %+v", value)
	}
	if err := StartIndependent(nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil command = %v", err)
	}
	if err := StartIndependent(exec.Command("relative-image")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("relative command = %v", err)
	}
	if err := StartAtLogin("unused"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("Windows login recovery was reported supported")
	}
	if err := StartRecovery("unused", "unused"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("Windows service recovery was reported supported")
	}
}

func TestLinuxIndependentChild(t *testing.T) {
	path := os.Getenv("KRASIS_LINUX_PROCESS_TEST")
	if path == "" {
		return
	}
	session, err := unix.Getsid(0)
	if err != nil {
		os.Exit(2)
	}
	data, _ := json.Marshal(struct{ PID, Session int }{os.Getpid(), session})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		os.Exit(2)
	}
}

func TestLinuxLiveDirectoriesIncludesSelf(t *testing.T) {
	paths, err := LiveDirectories()
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	for _, path := range paths {
		if path == filepath.Dir(self) {
			return
		}
	}
	t.Fatal("current process directory missing from liveness result")
}
