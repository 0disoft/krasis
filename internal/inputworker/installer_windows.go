//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/installationdiag"
)

// InstallationResult contains no native error, path, identity or input data.
type InstallationResult uint8

const (
	InstallationOK               InstallationResult = 0
	InstallationDenied           InstallationResult = 3
	InstallationRestored         InstallationResult = 4
	InstallationRecoveryRequired InstallationResult = 5
)

// ManageEmbeddedInstallation is for the attended, elevated installer only.
// Pins and images must come from the installer's compiled payload, never RPC,
// a received manifest or caller-selected disk paths. The backend independently
// checks elevation, protected ownership and the fixed SCM configuration.
func ManageEmbeddedInstallation(ctx context.Context, remove bool, agent, input []byte, agentPin, inputPin [sha256.Size]byte) InstallationResult {
	return ManageEmbeddedInstallationDetailed(ctx, remove, agent, input, agentPin, inputPin).Result
}

type InstallationReport struct {
	Result     InstallationResult
	Diagnostic installationdiag.Summary
}

func ManageEmbeddedInstallationDetailed(ctx context.Context, remove bool, agent, input []byte, agentPin, inputPin [sha256.Size]byte) InstallationReport {
	action := "install"
	if remove {
		action = "remove"
	}
	summary := installationdiag.Summary{Time: time.Now().UTC().Format(time.RFC3339Nano), Action: action, Result: uint8(InstallationDenied), Stage: "validation", Recovery: "not_attempted"}
	rejected := func(stage string, err error) InstallationReport {
		summary.Stage = stage
		summary.Operation, summary.Win32Error, summary.Win32Known = installErrorFields(err)
		return InstallationReport{InstallationDenied, summary}
	}
	if ctx == nil || ctx.Err() != nil {
		return rejected("validation", nil)
	}
	payload, err := pinnedInstallPayload(agent, input, installImages{Agent: agentPin, Service: inputPin})
	if err != nil {
		return rejected("payload", err)
	}
	backend, err := newWindowsInstallBackend(payload)
	if err != nil {
		return rejected("backend", err)
	}
	target := payload.images
	if remove {
		target = installImages{}
	}
	return runObservedInstallation(ctx, backend, target, summary, backend.Close)
}

func runObservedInstallation(ctx context.Context, backend installBackend, target installImages, summary installationdiag.Summary, closeBackend func() error) InstallationReport {
	observed := &observedInstallBackend{installBackend: backend, summary: summary}
	err := applyInputInstallation(ctx, observed, target)
	result := installationResult(err)
	summary = observed.summary
	if err == nil {
		summary.Stage, summary.Recovery = "complete", "not_needed"
	} else if errors.Is(err, errInstallFailed) {
		summary.Recovery = "restored"
	} else if errors.Is(err, errInstallJournal) {
		summary.Recovery = "journal_unknown"
	} else {
		summary.Recovery = "required"
	}
	if !observed.failed && err != nil {
		summary.Stage = "validation"
	}
	if closeErr := closeBackend(); closeErr != nil {
		result = InstallationRecoveryRequired
		summary.CloseOperation, summary.CloseWin32Error, summary.CloseWin32Known = installErrorFields(closeErr)
		if err == nil {
			summary.Stage = "close"
		}
	}
	summary.Result = uint8(result)
	return InstallationReport{result, summary}
}

func installationResult(err error) InstallationResult {
	switch {
	case err == nil:
		return InstallationOK
	case errors.Is(err, errInstallFailed):
		return InstallationRestored
	default:
		return InstallationRecoveryRequired
	}
}
