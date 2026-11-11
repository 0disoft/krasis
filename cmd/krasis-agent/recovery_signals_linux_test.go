//go:build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestRecoveryChildRetainsJournalUntilEOFDespiteTerminationSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(ctx, executable, "-test.run=^TestRecoverySignalChild$")
	child.Env = append(os.Environ(), "KRASIS_TEST_RECOVERY_SIGNAL_CHILD=1")
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	reader := bufio.NewReader(output)
	if ready, err := reader.ReadString('\n'); err != nil || ready != "ready\n" {
		t.Fatalf("child readiness %q: %v", ready, err)
	}
	for _, termination := range []os.Signal{syscall.SIGTERM, syscall.SIGHUP, os.Interrupt} {
		if err := child.Process.Signal(termination); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(input, "still-owned"); err != nil {
			t.Fatal(err)
		}
		if reply, err := reader.ReadString('\n'); err != nil || reply != "still-owned\n" {
			t.Fatalf("child lost its journal on %v: %q %v", termination, reply, err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, reader)
	if err := child.Wait(); err != nil {
		t.Fatalf("child did not exit normally after parent EOF: %v", err)
	}
}

func TestRecoverySignalChild(t *testing.T) {
	if os.Getenv("KRASIS_TEST_RECOVERY_SIGNAL_CHILD") != "1" {
		t.Skip("internal isolated signal fixture")
	}
	stop := preserveRecoveryUntilParentEOF()
	defer stop()
	fmt.Println("ready")
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		fmt.Println(input.Text())
	}
	if err := input.Err(); err != nil {
		t.Fatal(err)
	}
}
