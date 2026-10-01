package productcli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/launchrecord"
)

type relaunchResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	Generation    int    `json:"generation,omitempty"`
}

func relaunch(deps dependencies) (relaunchResult, int) {
	result := relaunchResult{SchemaVersion: "krasis.cli.relaunch/v1"}
	executable := deps.executable
	if executable == "" {
		executable, _ = os.Executable()
	}
	fallback := ""
	if executable != "" {
		fallback = filepath.Join(filepath.Dir(executable), "krasis.exe")
	}
	record, err := launchrecord.Relaunch(deps.localAppData, fallback, deps.launch)
	if err != nil {
		result.Code = "launch_failed"
		if errors.Is(err, launchrecord.ErrMissing) || errors.Is(err, os.ErrNotExist) {
			result.Code = "launch_record_missing"
		}
		if errors.Is(err, launchrecord.ErrInvalid) {
			result.Code = "launch_record_invalid"
		}
		return result, ExitUnsafeState
	}
	result.Code, result.Generation = "launch_requested", record.Generation
	return result, ExitOK
}
