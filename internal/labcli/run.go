package labcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	exitSuccess     = 0
	exitRuntime     = 1
	exitUsage       = 2
	exitUnsupported = 3
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return exitUsage
	}

	command := args[0]
	commandArgs := args[1:]
	switch command {
	case "help", "-h", "--help":
		writeUsage(stdout)
		return exitSuccess
	case "diagnose":
		return runDiagnose(ctx, commandArgs, stdout, stderr, platform)
	case "capture":
		return runCapture(ctx, commandArgs, stdout, stderr, platform)
	case "selftest-inject":
		return runSelfTestInject(ctx, commandArgs, stdout, stderr, platform)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", command)
		writeUsage(stderr)
		return exitUsage
	}
}

func runSelfTestInject(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	jsonOutput, ok := parseOutputFlag(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: krasis-lab selftest-inject [--json]")
		return exitUsage
	}

	result, err := platform.SelfTestInject(ctx)
	if err != nil {
		return writeError(stderr, err)
	}

	if jsonOutput {
		if exitCode := writeJSON(stdout, stderr, result); exitCode != exitSuccess {
			return exitCode
		}
	} else {
		fmt.Fprintf(stdout, "SendInput self-test: passed=%t sent=%d observed=%d pointer-restored=%t\n", result.Passed, result.Sent, result.Observed, result.PointerRestored)
		if result.FailureCode != "" {
			fmt.Fprintf(stdout, "Failure: %s\n", result.FailureCode)
		}
	}

	if !result.Passed {
		return exitRuntime
	}
	return exitSuccess
}

func runDiagnose(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	jsonOutput, ok := parseOutputFlag(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: krasis-lab diagnose [--json]")
		return exitUsage
	}

	diagnostics, err := platform.Diagnose(ctx)
	if err != nil {
		return writeError(stderr, err)
	}

	if jsonOutput {
		return writeJSON(stdout, stderr, diagnostics)
	}

	fmt.Fprintf(stdout, "Krasis input diagnostics\n")
	fmt.Fprintf(stdout, "  OS: %s/%s\n", diagnostics.OS, diagnostics.Architecture)
	fmt.Fprintf(stdout, "  Windows: %d.%d build %d\n", diagnostics.Windows.Major, diagnostics.Windows.Minor, diagnostics.Windows.Build)
	fmt.Fprintf(stdout, "  Session: %d\n", diagnostics.Process.SessionID)
	fmt.Fprintf(stdout, "  Elevated: %t\n", diagnostics.Process.Elevated)
	fmt.Fprintf(stdout, "  Virtual screen: left=%d top=%d width=%d height=%d monitors=%d\n",
		diagnostics.Display.Left,
		diagnostics.Display.Top,
		diagnostics.Display.Width,
		diagnostics.Display.Height,
		diagnostics.Display.MonitorCount,
	)
	fmt.Fprintf(stdout, "  Mouse buttons: %d (swapped=%t)\n", diagnostics.Display.MouseButtons, diagnostics.Display.SwapButtons)
	fmt.Fprintf(stdout, "  Capabilities: keyboard-hook=%t mouse-hook=%t raw-input=%t send-input=%t\n",
		diagnostics.Capabilities.LowLevelKeyboardHook,
		diagnostics.Capabilities.LowLevelMouseHook,
		diagnostics.Capabilities.RawInput,
		diagnostics.Capabilities.SendInput,
	)
	return exitSuccess
}

func runCapture(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parseCaptureArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid capture options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab capture [--json] [--duration DURATION]")
		return exitUsage
	}

	if !options.jsonOutput {
		fmt.Fprintln(stdout, "Capturing aggregate input counts. Raw keys and pointer coordinates are never printed.")
		if options.duration > 0 {
			fmt.Fprintf(stdout, "Capture stops automatically after %s.\n", options.duration)
		} else {
			fmt.Fprintln(stdout, "Press Ctrl+C to stop.")
		}
	}

	captureContext, cancelCapture := context.WithCancel(ctx)
	defer cancelCapture()
	cancelDuration := func() {}
	if options.duration > 0 {
		captureContext, cancelDuration = context.WithTimeout(captureContext, options.duration)
	}
	defer cancelDuration()

	var reportMu sync.Mutex
	var reportErr error
	err = platform.Capture(captureContext, lab.CaptureOptions{ReportInterval: time.Second}, func(snapshot lab.CaptureSnapshot) {
		reportMu.Lock()
		defer reportMu.Unlock()
		if reportErr != nil {
			return
		}
		if options.jsonOutput {
			reportErr = json.NewEncoder(stdout).Encode(snapshot)
		} else {
			reportErr = writeCaptureSnapshot(stdout, snapshot)
		}
		if reportErr != nil {
			cancelCapture()
		}
	})
	reportMu.Lock()
	outputErr := reportErr
	reportMu.Unlock()
	if outputErr != nil {
		fmt.Fprintf(stderr, "output_error: %v\n", outputErr)
		return exitRuntime
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return writeError(stderr, err)
	}
	return exitSuccess
}

type captureArgs struct {
	jsonOutput bool
	duration   time.Duration
}

func parseCaptureArgs(args []string) (captureArgs, error) {
	var parsed captureArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			if parsed.jsonOutput {
				return captureArgs{}, fmt.Errorf("--json may be specified only once")
			}
			parsed.jsonOutput = true
		case argument == "--duration":
			if index+1 >= len(args) {
				return captureArgs{}, fmt.Errorf("--duration requires a value")
			}
			index++
			duration, err := parsePositiveDuration(args[index])
			if err != nil {
				return captureArgs{}, err
			}
			if parsed.duration > 0 {
				return captureArgs{}, fmt.Errorf("--duration may be specified only once")
			}
			parsed.duration = duration
		case strings.HasPrefix(argument, "--duration="):
			duration, err := parsePositiveDuration(strings.TrimPrefix(argument, "--duration="))
			if err != nil {
				return captureArgs{}, err
			}
			if parsed.duration > 0 {
				return captureArgs{}, fmt.Errorf("--duration may be specified only once")
			}
			parsed.duration = duration
		default:
			return captureArgs{}, fmt.Errorf("unknown option %q", argument)
		}
	}
	return parsed, nil
}

func parsePositiveDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return duration, nil
}

func writeCaptureSnapshot(stdout io.Writer, snapshot lab.CaptureSnapshot) error {
	_, err := fmt.Fprintf(stdout,
		"elapsed=%s keyboard(physical=%d/%d injected=%d/%d other=%d) mouse(move=%d/%d button=%d/%d/%d/%d wheel=%d/%d other=%d)\n",
		snapshot.Elapsed.Round(time.Millisecond),
		snapshot.Keyboard.PhysicalDown,
		snapshot.Keyboard.PhysicalUp,
		snapshot.Keyboard.InjectedDown,
		snapshot.Keyboard.InjectedUp,
		snapshot.Keyboard.Other,
		snapshot.Mouse.PhysicalMove,
		snapshot.Mouse.InjectedMove,
		snapshot.Mouse.PhysicalButtonDown,
		snapshot.Mouse.PhysicalButtonUp,
		snapshot.Mouse.InjectedButtonDown,
		snapshot.Mouse.InjectedButtonUp,
		snapshot.Mouse.PhysicalWheel,
		snapshot.Mouse.InjectedWheel,
		snapshot.Mouse.Other,
	)
	return err
}

func parseOutputFlag(args []string) (bool, bool) {
	if len(args) == 0 {
		return false, true
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, true
	}
	return false, false
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "output_error: %v\n", err)
		return exitRuntime
	}
	return exitSuccess
}

func writeError(stderr io.Writer, err error) int {
	if errors.Is(err, lab.ErrUnsupported) {
		fmt.Fprintf(stderr, "unsupported: %v\n", err)
		return exitUnsupported
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "unknown runtime error"
	}
	fmt.Fprintf(stderr, "runtime_error: %s\n", message)
	return exitRuntime
}

func writeUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage:")
	fmt.Fprintln(output, "  krasis-lab diagnose [--json]")
	fmt.Fprintln(output, "  krasis-lab capture [--json] [--duration DURATION]")
	fmt.Fprintln(output, "  krasis-lab selftest-inject [--json]")
	fmt.Fprintln(output, "  krasis-lab help")
}
