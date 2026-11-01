package windowsscript

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRunBoundsInheritedOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		waitDelay time.Duration
		want      error
	}{
		{"exit", 0, exec.ErrWaitDelay},
		{"cancel", 10 * time.Second, context.Canceled},
		{"deadline", 0, context.DeadlineExceeded},
		{"shorter", 20 * time.Millisecond, exec.ErrWaitDelay},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tc.name == "deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
			}
			defer cancel()
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInheritedOutputHelper$")
			command.Env = append(os.Environ(), "KRASIS_PIPE_TEST_MODE="+tc.name, "KRASIS_PIPE_TEST_PID="+pidFile, "GORACE=atexit_sleep_ms=0")
			command.WaitDelay = tc.waitDelay
			done := make(chan error, 1)
			go func() { done <- Run(ctx, command, "test registration") }()
			joined := false
			t.Cleanup(func() {
				cancel()
				if data, err := os.ReadFile(pidFile); err == nil {
					if pid, err := strconv.Atoi(string(data)); err == nil {
						if child, err := os.FindProcess(pid); err == nil {
							_ = child.Kill()
							_ = child.Release()
						}
					}
				}
				if !joined {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("helper did not stop after cleanup")
					}
				}
			})
			readyUntil := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(pidFile); err == nil {
					break
				}
				if time.Now().After(readyUntil) {
					t.Fatal("descendant did not acquire inherited output")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if tc.name == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				joined = true
				var failure *Failure
				if !errors.Is(err, tc.want) || !errors.As(err, &failure) || failure.Timeout != (tc.name == "deadline") {
					t.Fatalf("lost pipe failure or cancellation: %v", err)
				}
				if tc.name == "cancel" || tc.name == "deadline" {
					var exit *exec.ExitError
					if !errors.As(err, &exit) {
						t.Fatal("lost original process error", err)
					}
				}
				if tc.name == "shorter" && command.WaitDelay != tc.waitDelay {
					t.Fatal("extended caller's shorter drain budget")
				}
			case <-time.After(4 * time.Second):
				t.Fatal("inherited output outlived cancellation/exit budget")
			}
		})
	}
}

func TestInheritedOutputHelper(t *testing.T) {
	mode := os.Getenv("KRASIS_PIPE_TEST_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	if mode == "child" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestInheritedOutputHelper$")
	child.Env = append(os.Environ(), "KRASIS_PIPE_TEST_MODE=child")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("KRASIS_PIPE_TEST_PID"), []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
		_ = child.Process.Kill()
		os.Exit(3)
	}
	_ = child.Process.Release()
	if mode == "cancel" || mode == "deadline" {
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}
