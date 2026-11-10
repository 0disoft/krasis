//go:build linux

package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/0disoft/krasis/internal/productcli"
)

func RunProductCLI(ctx context.Context, args []string, stdout, stderr io.Writer, localData string) int {
	return runProductCLI(ctx, args, stdout, stderr, localData, EnsureAgent)
}

func runProductCLI(ctx context.Context, args []string, stdout, stderr io.Writer, localData string, ensure func(context.Context, string, string) error) int {
	command, jsonOutput, override := linuxCLICommand(args)
	explicit := len(command) == 1 && (command[0] == "start" || command[0] == "listen") || len(command) == 2 && command[0] == "agent" && command[1] == "start"
	if explicit {
		if override {
			fmt.Fprintln(stderr, "config_error: agent start does not use --control-dir")
			return productcli.ExitUsage
		}
		self, err := os.Executable()
		if err == nil {
			err = ensure(ctx, localData, self)
		}
		if err != nil {
			fmt.Fprintln(stderr, "agent_unavailable: Linux agent startup failed")
			return productcli.ExitAgentUnavailable
		}
		if jsonOutput {
			if err := json.NewEncoder(stdout).Encode(struct {
				SchemaVersion string `json:"schemaVersion"`
				Code          string `json:"code"`
			}{"krasis.agent-start/v1", "agent_ready"}); err != nil {
				return productcli.ExitInternal
			}
		} else if _, err := fmt.Fprintln(stdout, "Linux 사용자 에이전트가 준비됐어요."); err != nil {
			return productcli.ExitInternal
		}
		return productcli.ExitOK
	}
	return productcli.RunWithAgentStartup(ctx, args, stdout, stderr, localData, func(ctx context.Context) error {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		return ensure(ctx, localData, self)
	})
}

func linuxCLICommand(args []string) (command []string, jsonOutput, override bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			if jsonOutput {
				return nil, false, false // Let the product parser reject duplicate flags.
			}
			jsonOutput = true
		case "--control-dir":
			override = true
			i++
		default:
			command = append(command, args[i])
		}
	}
	return
}
