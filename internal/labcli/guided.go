package labcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/evidence"
	"github.com/0disoft/krasis/internal/lab"
)

type guidedProbeArgs struct {
	deviceAlias   string
	stageDuration time.Duration
	outputPath    string
}

type guidedStageDefinition struct {
	name        string
	instruction string
	evaluate    func(lab.CaptureSnapshot) []string
}

var guidedStageDefinitions = []guidedStageDefinition{
	{
		name:        "keyboard",
		instruction: "Press and release several keyboard keys.",
		evaluate: func(snapshot lab.CaptureSnapshot) []string {
			issues := make([]string, 0, 2)
			if snapshot.Keyboard.PhysicalDown == 0 {
				issues = append(issues, "keyboard_down_not_observed")
			}
			if snapshot.Keyboard.PhysicalUp == 0 {
				issues = append(issues, "keyboard_up_not_observed")
			}
			return issues
		},
	},
	{
		name:        "pointer",
		instruction: "Move the pointer with the touchpad or mouse.",
		evaluate: func(snapshot lab.CaptureSnapshot) []string {
			if snapshot.Mouse.PhysicalMove == 0 {
				return []string{"pointer_move_not_observed"}
			}
			return []string{}
		},
	},
	{
		name:        "wheel-buttons",
		instruction: "Scroll, then press and release a primary mouse or touchpad button.",
		evaluate: func(snapshot lab.CaptureSnapshot) []string {
			issues := make([]string, 0, 3)
			if snapshot.Mouse.PhysicalWheel == 0 {
				issues = append(issues, "wheel_not_observed")
			}
			if snapshot.Mouse.PhysicalButtonDown == 0 {
				issues = append(issues, "mouse_button_down_not_observed")
			}
			if snapshot.Mouse.PhysicalButtonUp == 0 {
				issues = append(issues, "mouse_button_up_not_observed")
			}
			return issues
		},
	},
}

func runGuidedProbe(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parseGuidedProbeArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid guided-probe options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab guided-probe --device H-A|H-B|S-A|S-B --stage-duration DURATION --output PATH.json")
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.outputPath); err != nil {
		fmt.Fprintf(stderr, "invalid guided-probe output: %v\n", err)
		return exitUsage
	}

	build := buildinfo.Read()
	report := lab.GuidedProbeReport{
		SchemaVersion:       lab.GuidedProbeSchemaVersion,
		DeviceAlias:         options.deviceAlias,
		CollectedAt:         time.Now().UTC(),
		BuildRevision:       build.Revision,
		BuildModified:       build.Modified,
		StageDurationMillis: options.stageDuration.Milliseconds(),
		Stages:              make([]lab.GuidedProbeStage, 0, len(guidedStageDefinitions)),
	}
	finish := func(failureCode string, runtimeErr error) int {
		report.FailureCode = failureCode
		report.Completed = failureCode == ""
		if err := evidence.WriteJSONExclusive(options.outputPath, report); err != nil {
			fmt.Fprintf(stderr, "output_error: %v\n", err)
			return exitRuntime
		}
		fmt.Fprintf(stdout, "Guided probe report: %s\n", options.outputPath)
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

	selfTest, err := platform.SelfTestInject(ctx)
	var selfTestFailure error
	if err != nil {
		selfTestFailure = err
	} else {
		report.SelfTest = &selfTest
		if !selfTest.Passed {
			selfTestFailure = errors.New("SendInput self-test did not pass")
		}
	}

	inputIncomplete := false
	for index, definition := range guidedStageDefinitions {
		fmt.Fprintf(stdout, "Stage %d/%d (%s): %s Capture starts now and lasts %s.\n",
			index+1,
			len(guidedStageDefinitions),
			definition.name,
			definition.instruction,
			options.stageDuration,
		)
		snapshot, captureErr := captureGuidedStage(ctx, options.stageDuration, platform)
		if captureErr != nil {
			if ctx.Err() != nil {
				return finish("guided_probe_canceled", ctx.Err())
			}
			return finish("capture_failed", captureErr)
		}
		issues := definition.evaluate(snapshot)
		if len(issues) > 0 {
			inputIncomplete = true
		}
		report.Stages = append(report.Stages, lab.GuidedProbeStage{
			Name:         definition.name,
			Passed:       len(issues) == 0,
			FailureCodes: issues,
			Capture:      snapshot,
		})
	}

	if selfTestFailure != nil {
		return finish("selftest_failed", selfTestFailure)
	}
	if inputIncomplete {
		return finish("guided_input_incomplete", errors.New("one or more guided physical-input stages did not observe every required event class"))
	}
	return finish("", nil)
}

func captureGuidedStage(ctx context.Context, duration time.Duration, platform lab.Platform) (lab.CaptureSnapshot, error) {
	captureContext, cancelCapture := context.WithTimeout(ctx, duration)
	defer cancelCapture()
	var finalSnapshot lab.CaptureSnapshot
	err := platform.Capture(captureContext, lab.CaptureOptions{ReportInterval: time.Second}, func(snapshot lab.CaptureSnapshot) {
		finalSnapshot = snapshot
	})
	if ctx.Err() != nil {
		return finalSnapshot, ctx.Err()
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return finalSnapshot, err
	}
	if finalSnapshot.SchemaVersion == "" {
		return finalSnapshot, errors.New("capture completed without an aggregate snapshot")
	}
	return finalSnapshot, nil
}

func parseGuidedProbeArgs(args []string) (guidedProbeArgs, error) {
	var parsed guidedProbeArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, hasInlineValue := strings.Cut(argument, "=")
		switch name {
		case "--device", "--stage-duration", "--output":
			if !hasInlineValue {
				if index+1 >= len(args) {
					return guidedProbeArgs{}, fmt.Errorf("%s requires a value", name)
				}
				index++
				value = args[index]
			}
			if value == "" {
				return guidedProbeArgs{}, fmt.Errorf("%s requires a value", name)
			}
		default:
			return guidedProbeArgs{}, fmt.Errorf("unknown option %q", argument)
		}

		switch name {
		case "--device":
			if parsed.deviceAlias != "" {
				return guidedProbeArgs{}, errors.New("--device may be specified only once")
			}
			if !lab.ValidProbeDeviceAlias(value) {
				return guidedProbeArgs{}, fmt.Errorf("unsupported device alias %q", value)
			}
			parsed.deviceAlias = value
		case "--stage-duration":
			if parsed.stageDuration > 0 {
				return guidedProbeArgs{}, errors.New("--stage-duration may be specified only once")
			}
			duration, err := parsePositiveDuration(value)
			if err != nil {
				return guidedProbeArgs{}, err
			}
			parsed.stageDuration = duration
		case "--output":
			if parsed.outputPath != "" {
				return guidedProbeArgs{}, errors.New("--output may be specified only once")
			}
			parsed.outputPath = value
		}
	}
	if parsed.deviceAlias == "" || parsed.stageDuration == 0 || parsed.outputPath == "" {
		return guidedProbeArgs{}, errors.New("--device, --stage-duration, and --output are required")
	}
	return parsed, nil
}
