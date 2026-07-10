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

	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/evidence"
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
	case "probe":
		return runProbe(ctx, commandArgs, stdout, stderr, platform)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", command)
		writeUsage(stderr)
		return exitUsage
	}
}

type probeArgs struct {
	deviceAlias string
	duration    time.Duration
	outputPath  string
}

var validProbeDeviceAliases = map[string]struct{}{
	"H-A": {},
	"H-B": {},
	"S-A": {},
	"S-B": {},
}

func runProbe(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parseProbeArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid probe options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab probe --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json")
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.outputPath); err != nil {
		fmt.Fprintf(stderr, "invalid probe output: %v\n", err)
		return exitUsage
	}

	build := buildinfo.Read()
	report := lab.ProbeReport{
		SchemaVersion:       lab.ProbeSchemaVersion,
		DeviceAlias:         options.deviceAlias,
		CollectedAt:         time.Now().UTC(),
		BuildRevision:       build.Revision,
		BuildModified:       build.Modified,
		RequestedDurationMs: options.duration.Milliseconds(),
	}
	finish := func(failureCode string, runtimeErr error) int {
		report.FailureCode = failureCode
		report.Completed = failureCode == ""
		if err := evidence.WriteJSONExclusive(options.outputPath, report); err != nil {
			fmt.Fprintf(stderr, "output_error: %v\n", err)
			return exitRuntime
		}
		fmt.Fprintf(stdout, "Probe report: %s\n", options.outputPath)
		if runtimeErr != nil {
			fmt.Fprintf(stderr, "runtime_error: %s\n", strings.TrimSpace(runtimeErr.Error()))
		}
		if failureCode != "" {
			return exitRuntime
		}
		return exitSuccess
	}

	diagnostics, err := platform.Diagnose(ctx)
	if err != nil {
		return finish("diagnose_failed", err)
	}
	report.Diagnostics = &diagnostics

	fmt.Fprintf(stdout, "Probe %s: move the pointer, use the wheel and buttons, and type during the %s capture.\n", options.deviceAlias, options.duration)
	captureContext, cancelCapture := context.WithTimeout(ctx, options.duration)
	var finalSnapshot lab.CaptureSnapshot
	err = platform.Capture(captureContext, lab.CaptureOptions{ReportInterval: time.Second}, func(snapshot lab.CaptureSnapshot) {
		finalSnapshot = snapshot
	})
	cancelCapture()
	if finalSnapshot.SchemaVersion != "" {
		report.Capture = &finalSnapshot
	}
	if ctx.Err() != nil {
		return finish("probe_canceled", ctx.Err())
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return finish("capture_failed", err)
	}
	if report.Capture == nil {
		return finish("capture_missing", errors.New("capture completed without an aggregate snapshot"))
	}

	selfTest, err := platform.SelfTestInject(ctx)
	if err != nil {
		return finish("selftest_failed", err)
	}
	report.SelfTest = &selfTest
	if !selfTest.Passed {
		return finish("selftest_failed", errors.New("SendInput self-test did not pass"))
	}
	return finish("", nil)
}

func parseProbeArgs(args []string) (probeArgs, error) {
	var parsed probeArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, hasInlineValue := strings.Cut(argument, "=")
		switch name {
		case "--device", "--duration", "--output":
			if !hasInlineValue {
				if index+1 >= len(args) {
					return probeArgs{}, fmt.Errorf("%s requires a value", name)
				}
				index++
				value = args[index]
			}
			if value == "" {
				return probeArgs{}, fmt.Errorf("%s requires a value", name)
			}
		default:
			return probeArgs{}, fmt.Errorf("unknown option %q", argument)
		}

		switch name {
		case "--device":
			if parsed.deviceAlias != "" {
				return probeArgs{}, fmt.Errorf("--device may be specified only once")
			}
			if _, ok := validProbeDeviceAliases[value]; !ok {
				return probeArgs{}, fmt.Errorf("unsupported device alias %q", value)
			}
			parsed.deviceAlias = value
		case "--duration":
			if parsed.duration > 0 {
				return probeArgs{}, fmt.Errorf("--duration may be specified only once")
			}
			duration, err := parsePositiveDuration(value)
			if err != nil {
				return probeArgs{}, err
			}
			parsed.duration = duration
		case "--output":
			if parsed.outputPath != "" {
				return probeArgs{}, fmt.Errorf("--output may be specified only once")
			}
			parsed.outputPath = value
		}
	}
	if parsed.deviceAlias == "" || parsed.duration == 0 || parsed.outputPath == "" {
		return probeArgs{}, errors.New("--device, --duration, and --output are required")
	}
	return parsed, nil
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
	fmt.Fprintln(output, "  krasis-lab probe --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json")
	fmt.Fprintln(output, "  krasis-lab help")
}
