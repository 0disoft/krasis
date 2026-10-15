package productcli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
)

type appRepairResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	Generation    int    `json:"generation,omitempty"`
	Stage         string `json:"stage,omitempty"`
}

// Only the verified adjacent product bundle can become a recovery target.
// No process launch, agent request, caller path or security-policy change.
func repairApp(deps dependencies) (appRepairResult, int) {
	executable := deps.executable
	if executable == "" {
		executable, _ = os.Executable()
	}
	bundle, code := verifyCurrentBundle(executable)
	if code != ExitOK {
		return appRepairResult{SchemaVersion: "krasis.cli.app-repair/v1", Code: bundle.Code}, code
	}
	return registerAppRecovery(deps.localAppData, executable, bundle, localVersion(), prepareExistingRecoveryTask)
}

// Seed ownership while a pre-upgrade launch record still names the scheduler
// target. First install may inspect absence, but cannot adopt an orphan task.
func prepareExistingRecoveryTask(base string) error {
	if _, err := launchrecord.Load(base); errors.Is(err, launchrecord.ErrMissing) {
		return appsettings.ValidateRecoveryTaskRegistration(base)
	} else if err != nil {
		return err
	}
	settings, err := appsettings.Load(base)
	if err != nil {
		return err
	}
	return appsettings.SetRecoveryTask(base, settings.StartAtLogin)
}

func registerAppRecovery(base, executable string, bundle updateVerifyResult, version versionResult, prepare ...func(string) error) (appRepairResult, int) {
	result := appRepairResult{SchemaVersion: "krasis.cli.app-repair/v1", Code: "bundle_version_mismatch"}
	if !bundle.Valid || bundle.Generation != version.CLIGeneration || !validHex(bundle.Revision, 40) || bundle.Revision != version.CLIBuild {
		return result, ExitUnsafeState
	}
	if len(prepare) != 0 && prepare[0] != nil {
		if err := prepare[0](base); err != nil {
			result.Code, result.Stage = "registration_failed", "task"
			return result, ExitUnsafeState
		}
	}
	self := filepath.Join(filepath.Dir(executable), "krasis.exe")
	if err := launchrecord.Register(base, self, bundle.Generation, bundle.Revision); err != nil {
		result.Code, result.Stage = "registration_failed", launchrecord.RegistrationStage(err)
		return result, ExitUnsafeState
	}
	result.Code, result.Generation = "recovery_ready", bundle.Generation
	return result, ExitOK
}
