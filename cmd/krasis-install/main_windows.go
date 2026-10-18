//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/installationdiag"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/productlog"
)

func main() {
	saveLogin := func(s logindiag.Summary) error {
		return productlog.SaveLoginConfiguration(os.Getenv("LOCALAPPDATA"), s)
	}
	if code, handled := runLoginConfigurationTracked(os.Args[1:], os.Stdout, inputworker.ConfigureLoginGrant, inputworker.RemoveLoginGrant, saveLogin); handled {
		os.Exit(code)
	}
	save := func(s installationdiag.Summary) error {
		return productlog.SaveInstallation(os.Getenv("LOCALAPPDATA"), s)
	}
	apply := func(ctx context.Context, action string, a, i []byte, ap, ip [32]byte) inputworker.InstallationReport {
		report := func(s installationdiag.Summary) { _ = save(s) }
		if action == "--repair" {
			var id [16]byte
			_, err := rand.Read(id[:])
			if err == nil {
				_, err = productlog.ExportForRequest(os.Getenv("LOCALAPPDATA"), hex.EncodeToString(id[:]))
			}
			if err != nil {
				native := logindiag.New("prepare", 3, err)
				return inputworker.InstallationReport{Result: inputworker.InstallationDenied, Diagnostic: installationdiag.Summary{
					Time: time.Now().UTC().Format(time.RFC3339Nano), Action: "install", Result: 3, Stage: "validation", Recovery: "not_attempted",
					Win32Error: native.Win32Error, Win32Known: native.Win32Known,
				}}
			}
			return inputworker.RepairEmbeddedInstallationTracked(ctx, a, i, ap, ip, report)
		}
		return inputworker.ManageEmbeddedInstallationTracked(ctx, action == "--remove", a, i, ap, ip, report)
	}
	os.Exit(runDetailed(os.Args[1:], os.Stdout, loadEmbeddedPayload, apply, save))
}

type installerPayload struct {
	agent, input       []byte
	agentPin, inputPin [32]byte
}

type installerApply func(context.Context, bool, []byte, []byte, [32]byte, [32]byte) inputworker.InstallationResult

func run(args []string, output io.Writer, load func() (installerPayload, error), apply installerApply) int {
	return runDetailed(args, output, load, func(ctx context.Context, action string, a, i []byte, ap, ip [32]byte) inputworker.InstallationReport {
		return inputworker.InstallationReport{Result: apply(ctx, action == "--remove", a, i, ap, ip)}
	}, nil)
}

func runDetailed(args []string, output io.Writer, load func() (installerPayload, error), apply func(context.Context, string, []byte, []byte, [32]byte, [32]byte) inputworker.InstallationReport, save func(installationdiag.Summary) error) int {
	code, state := 2, "invalid_arguments"
	var diagnostic *installationdiag.Summary
	var diagnosticSaved *bool
	if len(args) == 1 && args[0] == "--help" {
		code, state = 0, "usage: krasis-install.exe --install | --repair | --remove | --prepare-login PEER | --disable-login (login approval persists until disabled)"
	} else if len(args) == 1 && (args[0] == "--install" || args[0] == "--remove" || args[0] == "--repair") {
		code, state = 3, "installation_denied"
		payload, err := load()
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			report := apply(ctx, args[0], payload.agent, payload.input, payload.agentPin, payload.inputPin)
			cancel()
			if report.Diagnostic.Valid() {
				diagnostic = &report.Diagnostic
				if save != nil {
					ok := save(report.Diagnostic) == nil
					diagnosticSaved = &ok
				}
			}
			switch report.Result {
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
		Schema          string                    `json:"schemaVersion"`
		Code            int                       `json:"code"`
		State           string                    `json:"state"`
		Diagnostic      *installationdiag.Summary `json:"diagnostic,omitempty"`
		DiagnosticSaved *bool                     `json:"diagnosticSaved,omitempty"`
	}{"krasis.input.install-result/v1", code, state, diagnostic, diagnosticSaved}) != nil {
		// An unreadable receipt must not be reported as a confirmed success.
		return 6
	}
	return code
}
