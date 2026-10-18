//go:build windows

package crashguard

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestGuardStderrReadUsesRuntimePoller(t *testing.T) {
	reader, writer, err := overlappedGuardPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if err := reader.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal("stderr is not pollable:", err)
	}
	var data [1]byte
	if _, err := reader.Read(data[:]); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("idle asynchronous read did not time out:", err)
	}
	if err := reader.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("E")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(reader, data[:]); err != nil || data[0] != 'E' {
		t.Fatal("stderr data changed:", data, err)
	}
}

func TestGuardStderrDrainsAndKeepsMemoryLimit(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestGuardStderrHelper")
	command.Env = append(os.Environ(), "KRASIS_STDERR_HELPER=1")
	configureProcess(command)
	sink := &limitedBuffer{remaining: maxStderrBytes}
	started, finish, err := configureStderr(command, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		started()
		_ = finish()
		t.Fatal(err)
	}
	started()
	if err := waitProcess(command); err != nil {
		t.Fatal(err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	if len(sink.String()) != maxStderrBytes {
		t.Fatal("stderr was lost or exceeded its memory limit", len(sink.String()))
	}
}

func TestGuardStderrHelper(t *testing.T) {
	if os.Getenv("KRASIS_STDERR_HELPER") != "1" {
		return
	}
	var data [8192]byte
	for i := 0; i < 16; i++ {
		if _, err := os.Stderr.Write(data[:]); err != nil {
			t.Fatal(err)
		}
	}
}
