//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
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
	if ctx == nil || ctx.Err() != nil {
		return InstallationDenied
	}
	payload, err := pinnedInstallPayload(agent, input, installImages{Agent: agentPin, Service: inputPin})
	if err != nil {
		return InstallationDenied
	}
	backend, err := newWindowsInstallBackend(payload)
	if err != nil {
		return InstallationDenied
	}
	target := payload.images
	if remove {
		target = installImages{}
	}
	result := installationResult(applyInputInstallation(ctx, backend, target))
	if backend.Close() != nil {
		return InstallationRecoveryRequired
	}
	return result
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
