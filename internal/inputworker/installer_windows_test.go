//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
)

func TestInstallerRejectsBeforeOpeningProtectedRoot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	agent, input := []byte("untrusted"), []byte("untrusted")
	for _, ctx := range []context.Context{nil, ctx, context.Background()} {
		for _, remove := range []bool{false, true} {
			if result := ManageEmbeddedInstallation(ctx, remove, agent, input, sha256.Sum256(agent), sha256.Sum256(input)); result != InstallationDenied {
				t.Fatal("invalid payload reached installation backend")
			}
		}
	}
}

func TestInstallerPreservesRecoveryClassification(t *testing.T) {
	for _, example := range []struct {
		err    error
		result InstallationResult
	}{
		{nil, InstallationOK},
		{errInstallFailed, InstallationRestored},
		{errors.Join(errors.New("hidden native detail"), errInstallFailed), InstallationRestored},
		{errInstallJournal, InstallationRecoveryRequired},
		{errInstallRecovery, InstallationRecoveryRequired},
		{ErrAuthority, InstallationRecoveryRequired},
	} {
		if installationResult(example.err) != example.result {
			t.Fatal("unknown or incomplete transaction classified as success")
		}
	}
}
