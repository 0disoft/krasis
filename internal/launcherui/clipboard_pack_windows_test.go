//go:build windows

package launcherui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestClipboardFilePackerChild(t *testing.T) {
	switch os.Getenv("KRASIS_TEST_PACK_CHILD") {
	case "pack":
		os.Exit(RunClipboardFilePacker(os.Stdin, os.Stdout))
	case "blocked":
		os.Exit(RunClipboardFilePacker(clipboardPackBlockedReader{}, os.Stdout))
	case "owner":
		self, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		command := exec.CommandContext(context.Background(), self, "-test.run=^TestClipboardFilePackerChild$")
		command.Env = append(os.Environ(), "KRASIS_TEST_PACK_CHILD=blocked")
		_, _ = runClipboardFilePacker(context.Background(), command, []string{"unused"})
		os.Exit(2)
	}
}

type clipboardPackBlockedReader struct{}

func (clipboardPackBlockedReader) Read([]byte) (int, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		os.Exit(2)
	}
	defer windows.CloseHandle(event)
	if path := os.Getenv("KRASIS_TEST_PACK_PID"); path != "" {
		if os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
			os.Exit(2)
		}
	}
	// A native wait models file I/O that cannot observe Go context cancellation.
	_, _ = windows.WaitForSingleObject(event, windows.INFINITE)
	return 0, os.ErrClosed
}

func TestClipboardPackChildExitsWhenOwnerIsKilled(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "helper.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	owner := exec.CommandContext(ctx, self, "-test.run=^TestClipboardFilePackerChild$")
	owner.Env = append(os.Environ(), "KRASIS_TEST_PACK_CHILD=owner", "KRASIS_TEST_PACK_PID="+pidFile)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Wait()
	defer owner.Process.Kill()
	var pid int
	for deadline := time.Now().Add(5 * time.Second); pid == 0 && time.Now().Before(deadline); {
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if pid == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if pid == 0 {
		t.Fatal("helper did not enter native wait")
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	defer windows.TerminateProcess(child, 2) // Clean up the intentionally orphaned baseline too.
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	state, err := windows.WaitForSingleObject(child, 2000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("clipboard helper survived owner death", state, err)
	}
}

func TestClipboardPackChildRoundtripAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("copy"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, blocked := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		mode := "pack"
		if blocked {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
			mode = "blocked"
		}
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, self, "-test.run=^TestClipboardFilePackerChild$")
		command.Env = append(os.Environ(), "KRASIS_TEST_PACK_CHILD="+mode)
		started := time.Now()
		data, err := runClipboardFilePacker(ctx, command, []string{path})
		if blocked {
			if err == nil || len(data) != 0 || time.Since(started) > 2*time.Second {
				t.Fatal("blocked helper escaped cancellation", err)
			}
		} else if err != nil || len(data) == 0 {
			t.Fatal("pack failed", err)
		}
		cancel()
	}
}
