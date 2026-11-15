//go:build windows

package crashguard

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGuardStdoutUsesPollerAndClosesUnusedWriter(t *testing.T) {
	command := exec.Command(filepath.Join(t.TempDir(), "missing-guard.exe"))
	reader, started, err := configureStdout(command)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer started()
	file := reader.(*os.File)
	if err := file.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal("guard ACK reader is not pollable", err)
	}
	var data [1]byte
	if _, err := reader.Read(data[:]); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("idle guard read did not yield through runtime poller", err)
	}
	if err := file.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err == nil {
		t.Fatal("missing process unexpectedly started")
	}
	started()
	started() // Cleanup may also run after the failed Start path.
	if _, err := reader.Read(data[:]); !errors.Is(err, io.EOF) {
		t.Fatal("parent writer survived failed process startup", err)
	}
}

func TestGuardStdoutRetainsChildOutputAndEOF(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestGuardStdoutHelper$")
	command.Env = append(os.Environ(), "KRASIS_STDOUT_HELPER=1")
	configureProcess(command)
	reader, started, err := configureStdout(command)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer started()
	if err := reader.(*os.File).SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	started()
	data, readErr := io.ReadAll(io.LimitReader(reader, 65537))
	if readErr != nil {
		_ = command.Process.Kill()
	}
	if err := waitProcess(command); err != nil {
		t.Fatal(err)
	}
	if readErr != nil || !bytes.Equal(data, bytes.Repeat([]byte{0x42}, 65536)) {
		t.Fatal("child output was lost, changed or failed to reach EOF", len(data), readErr)
	}
}

func TestGuardStdoutCloseInterruptsOutstandingRead(t *testing.T) {
	reader, started, err := configureStdout(exec.Command(os.Args[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer started()
	defer reader.Close()
	done := make(chan error, 1)
	go func() { var data [1]byte; _, err := reader.Read(data[:]); done <- err }()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrClosed) {
			t.Fatal("pending guard read did not preserve close", err)
		}
	case <-time.After(time.Second):
		t.Fatal("closed guard reader did not join")
	}
}

func TestGuardStdoutHelper(t *testing.T) {
	if os.Getenv("KRASIS_STDOUT_HELPER") != "1" {
		return
	}
	if _, err := os.Stdout.Write(bytes.Repeat([]byte{0x42}, 65536)); err != nil {
		os.Exit(2)
	}
	os.Exit(0) // Do not append the Go test runner's PASS to the protocol stream.
}
