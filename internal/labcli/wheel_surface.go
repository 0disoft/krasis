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

const (
	wheelSurfacePathHookAndRaw = "HOOK_AND_RAW"
	wheelSurfacePathHookOnly   = "HOOK_ONLY"
	wheelSurfacePathRawOnly    = "RAW_ONLY"
	wheelSurfacePathNeither    = "NEITHER"
)

type wheelSurfaceArgs struct {
	deviceAlias string
	duration    time.Duration
	outputPath  string
}

func runWheelSurface(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parseWheelSurfaceArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid wheel-surface options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab wheel-surface --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json")
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.outputPath); err != nil {
		fmt.Fprintf(stderr, "invalid wheel-surface output: %v\n", err)
		return exitUsage
	}
	surface, supported := platform.(lab.WheelSurfacePlatform)
	if !supported {
		fmt.Fprintln(stderr, "unsupported: wheel surface requires Windows Raw Input, low-level mouse hooks, and SendInput")
		return exitUnsupported
	}

	build := buildinfo.Read()
	report := lab.WheelSurfaceReport{
		SchemaVersion:       lab.WheelSurfaceSchemaVersion,
		DeviceAlias:         options.deviceAlias,
		CollectedAt:         time.Now().UTC(),
		BuildRevision:       build.Revision,
		BuildModified:       build.Modified,
		RequestedDurationMs: options.duration.Milliseconds(),
		FailureCodes:        []string{},
	}
	finish := func(runtimeErr error) int {
		if err := evidence.WriteJSONExclusive(options.outputPath, report); err != nil {
			fmt.Fprintf(stderr, "output_error: %v\n", err)
			return exitRuntime
		}
		fmt.Fprintf(stdout, "Wheel surface report: %s\n", options.outputPath)
		if runtimeErr != nil {
			fmt.Fprintf(stderr, "runtime_error: %s\n", strings.TrimSpace(runtimeErr.Error()))
		}
		if !report.Completed || !report.Passed {
			return exitRuntime
		}
		return exitSuccess
	}

	diagnostics, err := platform.Diagnose(ctx)
	if err != nil {
		report.FailureCodes = append(report.FailureCodes, "diagnose_failed")
		return finish(err)
	}
	report.Diagnostics = &diagnostics

	fmt.Fprintf(stdout, "Wheel surface %s: for %s, move the pointer, click left and right, then use two-finger vertical and horizontal scrolling. Physical input is observed but not suppressed; only tiny tagged pointer-move health sentinels are injected and swallowed.\n",
		options.deviceAlias, options.duration)
	observation, captureErr := surface.WheelSurface(ctx, lab.WheelSurfaceOptions{Duration: options.duration})
	report.Observation = &observation
	report.Signature = wheelSurfaceSignature(observation)
	if ctx.Err() != nil {
		report.FailureCodes = append(report.FailureCodes, "wheel_surface_canceled")
		return finish(ctx.Err())
	}
	if captureErr != nil {
		report.FailureCodes = append(report.FailureCodes, "wheel_surface_capture_failed")
		return finish(captureErr)
	}

	report.Completed = true
	report.FailureCodes = wheelSurfaceEvidenceFailures(observation)
	report.Passed = len(report.FailureCodes) == 0
	return finish(nil)
}

func wheelSurfaceSignature(observation lab.WheelSurfaceObservation) lab.WheelSurfaceSignature {
	hookPhysicalWheel := observation.Hook.PhysicalVerticalWheels > 0 || observation.Hook.PhysicalHorizontalWheels > 0
	hookSynthesizedWheel := observation.Hook.SynthesizedVerticalWheels > 0 || observation.Hook.SynthesizedHorizontalWheels > 0
	hookWheel := hookPhysicalWheel || hookSynthesizedWheel
	rawWheel := observation.Raw.VerticalWheelPackets > 0 || observation.Raw.HorizontalWheelPackets > 0
	path := wheelSurfacePathNeither
	switch {
	case hookWheel && rawWheel:
		path = wheelSurfacePathHookAndRaw
	case hookWheel:
		path = wheelSurfacePathHookOnly
	case rawWheel:
		path = wheelSurfacePathRawOnly
	}
	return lab.WheelSurfaceSignature{
		HookHealthConfirmed:          wheelSurfaceSentinelHealthy(observation.Sentinel),
		PhysicalInteractionObserved:  wheelSurfacePhysicalInteractionObserved(observation.Hook),
		HookPhysicalWheelObserved:    hookPhysicalWheel,
		HookSynthesizedWheelObserved: hookSynthesizedWheel,
		HookWheelObserved:            hookWheel,
		RawWheelObserved:             rawWheel,
		ObservedPath:                 path,
	}
}

func wheelSurfaceEvidenceFailures(observation lab.WheelSurfaceObservation) []string {
	failures := make([]string, 0, 5)
	if observation.Sentinel.Requested == 0 {
		failures = append(failures, "sentinel_not_requested")
	}
	if observation.Sentinel.Inserted != observation.Sentinel.Requested {
		failures = append(failures, "sentinel_insertion_incomplete")
	}
	if observation.Sentinel.Observed != observation.Sentinel.Requested {
		failures = append(failures, "sentinel_not_observed")
	}
	if observation.Sentinel.Swallowed != observation.Sentinel.Observed {
		failures = append(failures, "sentinel_not_swallowed")
	}
	if !wheelSurfacePhysicalInteractionObserved(observation.Hook) {
		failures = append(failures, "physical_interaction_not_observed")
	}
	return failures
}

func wheelSurfacePhysicalInteractionObserved(hook lab.WheelSurfaceHookCounts) bool {
	return hook.PhysicalMoves > 0 || hook.PhysicalButtonDowns > 0 || hook.PhysicalButtonUps > 0
}

func wheelSurfaceSentinelHealthy(sentinel lab.WheelSurfaceSentinelCounts) bool {
	return sentinel.Requested > 0 &&
		sentinel.Inserted == sentinel.Requested &&
		sentinel.Observed == sentinel.Requested &&
		sentinel.Swallowed == sentinel.Observed
}

func parseWheelSurfaceArgs(args []string) (wheelSurfaceArgs, error) {
	var parsed wheelSurfaceArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		switch name {
		case "--device", "--duration", "--output":
			if !inline {
				if index+1 >= len(args) {
					return wheelSurfaceArgs{}, fmt.Errorf("%s requires a value", name)
				}
				index++
				value = args[index]
			}
			if value == "" {
				return wheelSurfaceArgs{}, fmt.Errorf("%s requires a value", name)
			}
		default:
			return wheelSurfaceArgs{}, fmt.Errorf("unknown option %q", argument)
		}

		switch name {
		case "--device":
			if parsed.deviceAlias != "" {
				return wheelSurfaceArgs{}, errors.New("--device may be specified only once")
			}
			if !lab.ValidProbeDeviceAlias(value) {
				return wheelSurfaceArgs{}, fmt.Errorf("unsupported device alias %q", value)
			}
			parsed.deviceAlias = value
		case "--duration":
			if parsed.duration > 0 {
				return wheelSurfaceArgs{}, errors.New("--duration may be specified only once")
			}
			duration, err := parsePositiveDuration(value)
			if err != nil {
				return wheelSurfaceArgs{}, err
			}
			parsed.duration = duration
		case "--output":
			if parsed.outputPath != "" {
				return wheelSurfaceArgs{}, errors.New("--output may be specified only once")
			}
			parsed.outputPath = value
		}
	}
	if parsed.deviceAlias == "" || parsed.duration == 0 || parsed.outputPath == "" {
		return wheelSurfaceArgs{}, errors.New("--device, --duration, and --output are required")
	}
	return parsed, nil
}
