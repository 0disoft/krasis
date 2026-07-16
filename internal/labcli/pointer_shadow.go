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

type pointerShadowArgs struct {
	deviceAlias string
	edge        string
	duration    time.Duration
	outputPath  string
}

func runPointerShadow(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parsePointerShadowArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid pointer-shadow options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab pointer-shadow --device H-A|H-B|S-A|S-B --edge LEFT|RIGHT --duration DURATION --output PATH.json")
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.outputPath); err != nil {
		fmt.Fprintf(stderr, "invalid pointer-shadow output: %v\n", err)
		return exitUsage
	}
	shadow, supported := platform.(lab.PointerShadowPlatform)
	if !supported {
		fmt.Fprintln(stderr, "unsupported: pointer shadow requires Windows Raw Input and low-level mouse hooks")
		return exitUnsupported
	}

	build := buildinfo.Read()
	report := lab.PointerShadowReport{
		SchemaVersion:       lab.PointerShadowSchemaVersion,
		DeviceAlias:         options.deviceAlias,
		Edge:                options.edge,
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
		fmt.Fprintf(stdout, "Pointer shadow report: %s\n", options.outputPath)
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

	fmt.Fprintf(stdout, "Pointer shadow %s/%s: move to the %s edge, then keep moving outward for %s. Input is observed but not suppressed or forwarded.\n",
		options.deviceAlias, options.edge, strings.ToLower(options.edge), options.duration)
	captureContext, cancelCapture := context.WithTimeout(ctx, options.duration)
	observation, captureErr := shadow.PointerShadow(captureContext, lab.PointerShadowOptions{
		Duration: options.duration,
		Edge:     options.edge,
	})
	cancelCapture()
	report.Observation = &observation
	report.Signature = pointerShadowSignature(observation)
	if ctx.Err() != nil {
		report.FailureCodes = append(report.FailureCodes, "pointer_shadow_canceled")
		return finish(ctx.Err())
	}
	if captureErr != nil && !errors.Is(captureErr, context.DeadlineExceeded) {
		report.FailureCodes = append(report.FailureCodes, "pointer_shadow_capture_failed")
		return finish(captureErr)
	}

	report.Completed = true
	report.FailureCodes = pointerShadowEvidenceFailures(observation, report.Signature)
	report.Passed = len(report.FailureCodes) == 0
	return finish(nil)
}

func pointerShadowSignature(observation lab.PointerShadowObservation) lab.PointerShadowSignature {
	signature := lab.PointerShadowSignature{
		RawOutwardAtEdge:       observation.Raw.OutwardWhileHookAtEdge > 0,
		LegacyXUnchangedAtEdge: observation.Hook.UnchangedAtEdgeMoves > 0,
		LegacyOutwardDeficit:   observation.Raw.OutwardWhileHookAtEdge > observation.Hook.OutwardAtEdgeMoves,
	}
	signature.Observed = signature.RawOutwardAtEdge && (signature.LegacyXUnchangedAtEdge || signature.LegacyOutwardDeficit)
	return signature
}

func pointerShadowEvidenceFailures(observation lab.PointerShadowObservation, signature lab.PointerShadowSignature) []string {
	failures := make([]string, 0, 6)
	if observation.Raw.RelativePackets == 0 {
		failures = append(failures, "raw_relative_not_observed")
	}
	if observation.Hook.PhysicalMoves == 0 {
		failures = append(failures, "hook_physical_move_not_observed")
	}
	if observation.Hook.InjectedMoves > 0 {
		failures = append(failures, "injected_pointer_interference")
	}
	if observation.Hook.EdgeMoves == 0 {
		failures = append(failures, "edge_not_observed")
	}
	if !signature.RawOutwardAtEdge {
		failures = append(failures, "raw_outward_at_edge_not_observed")
	}
	if !signature.Observed {
		failures = append(failures, "legacy_edge_loss_not_observed")
	}
	return failures
}

func parsePointerShadowArgs(args []string) (pointerShadowArgs, error) {
	var parsed pointerShadowArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		switch name {
		case "--device", "--edge", "--duration", "--output":
			if !inline {
				if index+1 >= len(args) {
					return pointerShadowArgs{}, fmt.Errorf("%s requires a value", name)
				}
				index++
				value = args[index]
			}
			if value == "" {
				return pointerShadowArgs{}, fmt.Errorf("%s requires a value", name)
			}
		default:
			return pointerShadowArgs{}, fmt.Errorf("unknown option %q", argument)
		}

		switch name {
		case "--device":
			if parsed.deviceAlias != "" {
				return pointerShadowArgs{}, errors.New("--device may be specified only once")
			}
			if !lab.ValidProbeDeviceAlias(value) {
				return pointerShadowArgs{}, fmt.Errorf("unsupported device alias %q", value)
			}
			parsed.deviceAlias = value
		case "--edge":
			if parsed.edge != "" {
				return pointerShadowArgs{}, errors.New("--edge may be specified only once")
			}
			parsed.edge = strings.ToUpper(value)
			if parsed.edge != "LEFT" && parsed.edge != "RIGHT" {
				return pointerShadowArgs{}, errors.New("--edge must be LEFT or RIGHT")
			}
		case "--duration":
			if parsed.duration > 0 {
				return pointerShadowArgs{}, errors.New("--duration may be specified only once")
			}
			duration, err := parsePositiveDuration(value)
			if err != nil {
				return pointerShadowArgs{}, err
			}
			parsed.duration = duration
		case "--output":
			if parsed.outputPath != "" {
				return pointerShadowArgs{}, errors.New("--output may be specified only once")
			}
			parsed.outputPath = value
		}
	}
	if parsed.deviceAlias == "" || parsed.edge == "" || parsed.duration == 0 || parsed.outputPath == "" {
		return pointerShadowArgs{}, errors.New("--device, --edge, --duration, and --output are required")
	}
	return parsed, nil
}
