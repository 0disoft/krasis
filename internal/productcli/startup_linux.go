//go:build linux

package productcli

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

// RunWithAgentStartup starts the Linux agent at the existing client-opening
// boundary, after the ordinary CLI parser has validated the command. Probe only
// Runtime before starting; never retry an already dispatched state change.
func RunWithAgentStartup(ctx context.Context, args []string, stdout, stderr io.Writer, localData string, start func(context.Context) error) int {
	parsed, err := parseGlobalOptions(args)
	if err == nil && parsed.controlDir == "" && len(parsed.args) == 1 && parsed.args[0] == "help" {
		value := commandHelp()
		value.Commands = append(value.Commands,
			helpCommand{"start", "Start the per-user Linux agent", false, true},
			helpCommand{"listen", "Start the Linux agent for paired incoming sessions", false, true},
			helpCommand{"agent start", "Start the per-user Linux agent", false, true},
		)
		return writeOutput(stdout, stderr, value, ExitOK, parsed.json)
	}
	if err != nil || parsed.controlDir != "" || !linuxAutostartCommand(parsed.args) || start == nil {
		return Run(ctx, args, stdout, stderr, localData)
	}
	return run(ctx, args, stdout, stderr, dependencies{
		localAppData: localData,
		inspectHost:  inspectDoctorHost,
		openClient: func(control string) (apiClient, error) {
			client, err := localapi.OpenClient(control)
			if err == nil {
				probe, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
				_, err = client.Runtime(probe)
				cancel()
				if err == nil {
					return client, nil
				}
			}
			if !errors.Is(err, localapi.ErrUnavailable) && !errors.Is(err, localapi.ErrTransport) {
				return nil, err
			}
			if err := start(ctx); err != nil {
				return nil, errors.Join(localapi.ErrUnavailable, err)
			}
			return localapi.OpenClient(control)
		},
	})
}

func linuxAutostartCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "connect":
		return true
	case "pair":
		return len(args) > 1 && (args[1] == "listen" || args[1] == "dial")
	case "connection":
		return len(args) > 1 && args[1] == "set"
	}
	return false
}
