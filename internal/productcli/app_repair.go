package productcli

import (
	"os"
	"path/filepath"

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
	return registerAppRecovery(deps.localAppData, executable, bundle, localVersion())
}

func registerAppRecovery(base, executable string, bundle updateVerifyResult, version versionResult) (appRepairResult, int) {
	result := appRepairResult{SchemaVersion: "krasis.cli.app-repair/v1", Code: "bundle_version_mismatch"}
	if !bundle.Valid || bundle.Generation != version.CLIGeneration || !validHex(bundle.Revision, 40) || bundle.Revision != version.CLIBuild {
		return result, ExitUnsafeState
	}
	self := filepath.Join(filepath.Dir(executable), "krasis.exe")
	if err := launchrecord.Register(base, self, bundle.Generation, bundle.Revision); err != nil {
		result.Code, result.Stage = "registration_failed", launchrecord.RegistrationStage(err)
		return result, ExitUnsafeState
	}
	result.Code, result.Generation = "recovery_ready", bundle.Generation
	return result, ExitOK
}
