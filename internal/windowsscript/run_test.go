package windowsscript

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRunRetainsExitAndOnlyNumericDiagnostics(t *testing.T) {
	for _, mode := range []string{"diagnostic", "invalid", "oversized", "success"} {
		t.Run(mode, func(t *testing.T) {
			command := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestScriptProcessHelper$")
			command.Env = append(os.Environ(), "KRASIS_SCRIPT_TEST_MODE="+mode)
			err := Run(context.Background(), command, "test registration")
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var failure *Failure
			var exit *exec.ExitError
			if !errors.As(err, &failure) || !errors.As(err, &exit) || failure.ExitCode != 7 || failure.Timeout {
				t.Fatal("lost helper process outcome", err)
			}
			if mode == "diagnostic" {
				if failure.Stage != 4 || failure.HRESULT != -2147024891 || failure.Line != 29 {
					t.Fatal("lost numeric exception details", err)
				}
			} else if failure.Stage != 0 {
				t.Fatal("accepted malformed or truncated diagnostics", err)
			}
			if strings.Contains(err.Error(), "private-helper-output") || len(err.Error()) > 200 {
				t.Fatal("helper output escaped into the product error")
			}
		})
	}
}

func TestRunRetainsCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestScriptProcessHelper$")
	if err := Run(ctx, command, "canceled registration"); !errors.Is(err, context.Canceled) {
		t.Fatal("helper lost caller cancellation", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 0)
	defer cancel()
	command = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestScriptProcessHelper$")
	err := Run(ctx, command, "expired registration")
	var failure *Failure
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &failure) || !failure.Timeout || failure.ExitCode != -1 {
		t.Fatal("helper timeout became an ordinary COM failure", err)
	}
}

func TestScriptProcessHelper(t *testing.T) {
	mode := os.Getenv("KRASIS_SCRIPT_TEST_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	fmt.Fprintln(os.Stderr, "private-helper-output")
	switch mode {
	case "diagnostic":
		fmt.Println("KRASIS_HELPER_ERROR stage=4 hresult=-2147024891 line=29")
	case "invalid":
		fmt.Println("KRASIS_HELPER_ERROR stage=4 hresult=-2147024891 line=29 private-helper-output")
		fmt.Println("KRASIS_HELPER_ERROR stage=4 hresult=4294967295 line=29")
		fmt.Println("KRASIS_HELPER_ERROR stage=99 hresult=-1 line=29")
	case "oversized":
		fmt.Print(strings.Repeat("private-helper-output", 1000))
		fmt.Println("\nKRASIS_HELPER_ERROR stage=4 hresult=-2147024891 line=29")
	case "success":
		os.Exit(0)
	}
	os.Exit(7)
}
