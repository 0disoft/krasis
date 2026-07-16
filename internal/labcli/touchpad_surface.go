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
	touchpadSurfacePathPointerAndLegacy = "POINTER_AND_LEGACY"
	touchpadSurfacePathPointerOnly      = "POINTER_ONLY"
	touchpadSurfacePathLegacyOnly       = "LEGACY_ONLY"
	touchpadSurfacePathNeither          = "NEITHER"
)

type touchpadSurfaceArgs struct {
	deviceAlias string
	duration    time.Duration
	outputPath  string
}

func runTouchpadSurface(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parseTouchpadSurfaceArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid touchpad-surface options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab touchpad-surface --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json")
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.outputPath); err != nil {
		fmt.Fprintf(stderr, "invalid touchpad-surface output: %v\n", err)
		return exitUsage
	}
	surface, supported := platform.(lab.TouchpadSurfacePlatform)
	if !supported {
		fmt.Fprintln(stderr, "unsupported: touchpad surface requires Windows 11 Precision Touchpad pointer APIs")
		return exitUnsupported
	}

	build := buildinfo.Read()
	report := lab.TouchpadSurfaceReport{
		SchemaVersion:       lab.TouchpadSurfaceSchemaVersion,
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
		fmt.Fprintf(stdout, "Touchpad surface report: %s\n", options.outputPath)
		if runtimeErr != nil {
			fmt.Fprintf(stderr, "runtime_error: %s\n", strings.TrimSpace(runtimeErr.Error()))
		}
		if errors.Is(runtimeErr, lab.ErrUnsupported) {
			return exitUnsupported
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

	fmt.Fprintf(stdout, "Touchpad surface %s: a test window will open for %s. Keep it foreground and use two-finger vertical and horizontal scrolling inside it. Only aggregate message counts are recorded.\n",
		options.deviceAlias, options.duration)
	observation, captureErr := surface.TouchpadSurface(ctx, lab.TouchpadSurfaceOptions{Duration: options.duration})
	report.Observation = &observation
	report.Signature = touchpadSurfaceSignature(observation)
	if captureErr != nil {
		switch {
		case errors.Is(captureErr, lab.ErrUnsupported):
			report.FailureCodes = append(report.FailureCodes, "touchpad_api_unsupported")
		case ctx.Err() != nil:
			report.FailureCodes = append(report.FailureCodes, "touchpad_surface_canceled")
		default:
			report.FailureCodes = append(report.FailureCodes, "touchpad_surface_capture_failed")
		}
		return finish(captureErr)
	}

	report.Completed = observation.StopReason == "duration_elapsed"
	report.FailureCodes = touchpadSurfaceEvidenceFailures(observation)
	report.Passed = report.Completed && len(report.FailureCodes) == 0
	return finish(nil)
}

func touchpadSurfaceSignature(observation lab.TouchpadSurfaceObservation) lab.TouchpadSurfaceSignature {
	pointerObserved := observation.Pointer.TouchpadConfirmed > 0
	legacyObserved := observation.Legacy.VerticalWheels > 0 || observation.Legacy.HorizontalWheels > 0
	path := touchpadSurfacePathNeither
	switch {
	case pointerObserved && legacyObserved:
		path = touchpadSurfacePathPointerAndLegacy
	case pointerObserved:
		path = touchpadSurfacePathPointerOnly
	case legacyObserved:
		path = touchpadSurfacePathLegacyOnly
	}
	return lab.TouchpadSurfaceSignature{
		TouchpadPointerObserved: pointerObserved,
		LegacyWheelObserved:     legacyObserved,
		ObservedPath:            path,
	}
}

func touchpadSurfaceEvidenceFailures(observation lab.TouchpadSurfaceObservation) []string {
	failures := make([]string, 0, 7)
	if !observation.API.RegisterAvailable || !observation.API.PointerTypeAvailable {
		failures = append(failures, "touchpad_api_unavailable")
	}
	if !observation.API.Registered {
		failures = append(failures, "touchpad_window_not_registered")
	}
	if !observation.Window.Created {
		failures = append(failures, "touchpad_window_not_created")
	}
	if !observation.Window.ForegroundObserved {
		failures = append(failures, "touchpad_window_not_activated")
	}
	if observation.Window.Deactivations > 0 {
		failures = append(failures, "touchpad_window_foreground_lost")
	}
	if observation.Pointer.TouchpadConfirmed == 0 {
		failures = append(failures, "touchpad_pointer_not_observed")
	}
	if observation.Pointer.TypeQueries > 0 && observation.Pointer.TypeQueries == observation.Pointer.TypeQueryFailures {
		failures = append(failures, "touchpad_pointer_type_unresolved")
	}
	if observation.StopReason != "duration_elapsed" {
		failures = append(failures, "touchpad_surface_closed_early")
	}
	return failures
}

func parseTouchpadSurfaceArgs(args []string) (touchpadSurfaceArgs, error) {
	var parsed touchpadSurfaceArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		switch name {
		case "--device", "--duration", "--output":
			if !inline {
				if index+1 >= len(args) {
					return touchpadSurfaceArgs{}, fmt.Errorf("%s requires a value", name)
				}
				index++
				value = args[index]
			}
			if value == "" {
				return touchpadSurfaceArgs{}, fmt.Errorf("%s requires a value", name)
			}
		default:
			return touchpadSurfaceArgs{}, fmt.Errorf("unknown option %q", argument)
		}

		switch name {
		case "--device":
			if parsed.deviceAlias != "" {
				return touchpadSurfaceArgs{}, errors.New("--device may be specified only once")
			}
			if !lab.ValidProbeDeviceAlias(value) {
				return touchpadSurfaceArgs{}, fmt.Errorf("unsupported device alias %q", value)
			}
			parsed.deviceAlias = value
		case "--duration":
			if parsed.duration > 0 {
				return touchpadSurfaceArgs{}, errors.New("--duration may be specified only once")
			}
			duration, err := parsePositiveDuration(value)
			if err != nil {
				return touchpadSurfaceArgs{}, err
			}
			parsed.duration = duration
		case "--output":
			if parsed.outputPath != "" {
				return touchpadSurfaceArgs{}, errors.New("--output may be specified only once")
			}
			parsed.outputPath = value
		}
	}
	if parsed.deviceAlias == "" || parsed.duration == 0 || parsed.outputPath == "" {
		return touchpadSurfaceArgs{}, errors.New("--device, --duration, and --output are required")
	}
	return parsed, nil
}
