//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/inputworker"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, loadEmbeddedPayload, inputworker.ManageEmbeddedInstallation))
}

type installerPayload struct {
	agent, input       []byte
	agentPin, inputPin [32]byte
}

type installerApply func(context.Context, bool, []byte, []byte, [32]byte, [32]byte) inputworker.InstallationResult

func run(args []string, output io.Writer, load func() (installerPayload, error), apply installerApply) int {
	code, state := 2, "invalid_arguments"
	if len(args) == 1 && args[0] == "--help" {
		code, state = 0, "usage: krasis-install.exe --install | --remove"
	} else if len(args) == 1 && (args[0] == "--install" || args[0] == "--remove") {
		code, state = 3, "installation_denied"
		payload, err := load()
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			result := apply(ctx, args[0] == "--remove", payload.agent, payload.input, payload.agentPin, payload.inputPin)
			cancel()
			switch result {
			case inputworker.InstallationOK:
				code, state = 0, "installed"
				if args[0] == "--remove" {
					state = "removed"
				}
			case inputworker.InstallationDenied:
			case inputworker.InstallationRestored:
				code, state = 4, "previous_installation_restored"
			default:
				code, state = 5, "recovery_required"
			}
		}
	}
	if json.NewEncoder(output).Encode(struct {
		Schema string `json:"schemaVersion"`
		Code   int    `json:"code"`
		State  string `json:"state"`
	}{"krasis.input.install-result/v1", code, state}) != nil {
		// An unreadable receipt must not be reported as a confirmed success.
		return 6
	}
	return code
}
