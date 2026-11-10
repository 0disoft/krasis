//go:build linux

package agentinstance

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLinuxInstanceExclusiveAcrossProcessesAndReleasedOnExit(t *testing.T) {
	name := fmt.Sprintf(`Local\Krasis-Agent-Test-%d-%s`, os.Getpid(), t.Name())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLinuxInstanceChild$")
	command.Env = append(os.Environ(), "KRASIS_LINUX_INSTANCE_TEST="+name)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); command.Wait() }()
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatalf("child did not acquire instance: %s, %v", scanner.Text(), scanner.Err())
	}
	if _, err := acquire(name); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second process acquired ownership: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	lease, err := acquire(name)
	if err != nil {
		t.Fatalf("crashed child retained ownership: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxInstanceChild(t *testing.T) {
	name := os.Getenv("KRASIS_LINUX_INSTANCE_TEST")
	if name == "" {
		return
	}
	lease, err := acquire(name)
	if err != nil {
		os.Exit(2)
	}
	defer lease.Close()
	fmt.Fprintln(os.Stdout, "locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestLinuxInstanceRejectsSymlinkAndUnsafeLock(t *testing.T) {
	base := t.TempDir()
	dir, err := unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dir)
	name := `Local\Krasis-Agent-Test-unsafe`
	path := filepath.Join(base, fmt.Sprintf("%x.lock", sha256.Sum256([]byte(name))))
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if lease, err := acquireAt(dir, name); err == nil {
		lease.Close()
		t.Fatal("symlink lock accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if lease, err := acquireAt(dir, name); err == nil {
		lease.Close()
		t.Fatal("non-private lock accepted")
	}
	if _, err := acquire("../../unsafe"); err == nil {
		t.Fatal("invalid instance name accepted")
	}
}
