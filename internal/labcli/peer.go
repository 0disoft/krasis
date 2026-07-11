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
	"github.com/0disoft/krasis/internal/peerprobe"
)

type peerProbeArgs struct {
	role       peerprobe.Role
	device     string
	address    string
	serverName string
	ca         string
	cert       string
	key        string
	output     string
	timeout    time.Duration
}

func runPeerProbe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	options, err := parsePeerProbeArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid peer-probe options: %v\n", err)
		writePeerProbeUsage(stderr)
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.output); err != nil {
		fmt.Fprintf(stderr, "invalid peer-probe output: %v\n", err)
		return exitUsage
	}
	build := buildinfo.Read()
	result, runErr := peerprobe.Run(ctx, peerprobe.Config{
		Role: options.role, DeviceAlias: options.device,
		BuildRevision: build.Revision, BuildModified: build.Modified,
		Address: options.address, ExpectedServerName: options.serverName,
		CAPath: options.ca, CertificatePath: options.cert, PrivateKeyPath: options.key,
		Timeout: options.timeout,
		Ready:   func(address string) { fmt.Fprintf(stdout, "Peer probe ready: %s\n", address) },
	})
	if err := evidence.WriteJSONExclusive(options.output, result); err != nil {
		fmt.Fprintf(stderr, "output_error: %v\n", err)
		return exitRuntime
	}
	fmt.Fprintf(stdout, "Peer probe report: %s\n", options.output)
	if runErr != nil {
		fmt.Fprintf(stderr, "runtime_error: %s\n", strings.TrimSpace(runErr.Error()))
		return exitRuntime
	}
	return exitSuccess
}

func parsePeerProbeArgs(args []string) (peerProbeArgs, error) {
	var parsed peerProbeArgs
	if len(args) == 0 {
		return parsed, errors.New("listen or connect mode is required")
	}
	parsed.role = peerprobe.Role(args[0])
	if parsed.role != peerprobe.RoleListen && parsed.role != peerprobe.RoleConnect {
		return peerProbeArgs{}, fmt.Errorf("unknown mode %q", args[0])
	}
	values := map[string]*string{
		"--device": &parsed.device, "--address": &parsed.address,
		"--server-name": &parsed.serverName, "--ca": &parsed.ca,
		"--cert": &parsed.cert, "--key": &parsed.key, "--output": &parsed.output,
	}
	for index := 1; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		if name == "--timeout" {
			if !inline {
				if index+1 >= len(args) {
					return peerProbeArgs{}, errors.New("--timeout requires a value")
				}
				index++
				value = args[index]
			}
			if parsed.timeout != 0 {
				return peerProbeArgs{}, errors.New("--timeout may be specified only once")
			}
			duration, err := parsePositiveDuration(value)
			if err != nil {
				return peerProbeArgs{}, err
			}
			parsed.timeout = duration
			continue
		}
		target, known := values[name]
		if !known {
			return peerProbeArgs{}, fmt.Errorf("unknown option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return peerProbeArgs{}, fmt.Errorf("%s requires a value", name)
			}
			index++
			value = args[index]
		}
		if value == "" {
			return peerProbeArgs{}, fmt.Errorf("%s requires a value", name)
		}
		if *target != "" {
			return peerProbeArgs{}, fmt.Errorf("%s may be specified only once", name)
		}
		*target = value
	}
	if !lab.ValidProbeDeviceAlias(parsed.device) {
		return peerProbeArgs{}, fmt.Errorf("unsupported device alias %q", parsed.device)
	}
	if parsed.address == "" || parsed.ca == "" || parsed.cert == "" || parsed.key == "" || parsed.output == "" {
		return peerProbeArgs{}, errors.New("--device, --address, --ca, --cert, --key, and --output are required")
	}
	if parsed.role == peerprobe.RoleConnect && parsed.serverName == "" {
		return peerProbeArgs{}, errors.New("--server-name is required in connect mode")
	}
	if parsed.role == peerprobe.RoleListen && parsed.serverName != "" {
		return peerProbeArgs{}, errors.New("--server-name is only valid in connect mode")
	}
	if parsed.timeout == 0 {
		parsed.timeout = 2 * time.Minute
	}
	return parsed, nil
}

func writePeerProbeUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: krasis-lab peer-probe listen --device ALIAS --address ADDRESS --ca CA.pem --cert CERT.pem --key KEY.pem --output REPORT.json [--timeout DURATION]")
	fmt.Fprintln(output, "       krasis-lab peer-probe connect --device ALIAS --address ADDRESS --server-name NAME --ca CA.pem --cert CERT.pem --key KEY.pem --output REPORT.json [--timeout DURATION]")
}
