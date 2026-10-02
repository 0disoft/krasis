package productcli

import (
	"errors"
	"strings"

	"github.com/0disoft/krasis/internal/launchrecord"
)

// Process presence and launch-record verification are independent observations.
func doctorUIComponent(uis []string, unknown bool, record launchrecord.Record, recordErr error, imageBuild func(string) string) doctorComponent {
	if unknown || len(uis) > 1 {
		return doctorComponent{State: "UNKNOWN"}
	}
	if len(uis) == 0 {
		return doctorComponent{State: "NOT_RUNNING"}
	}
	ui := doctorComponent{State: "RUNNING_UNVERIFIED", Build: imageBuild(uis[0]), PathStatus: "UNKNOWN"}
	if recordErr != nil {
		if errors.Is(recordErr, launchrecord.ErrInvalid) {
			ui.PathStatus = "MISMATCH"
		}
		return ui
	}
	if !strings.EqualFold(uis[0], record.Executable) || ui.Build == "" || ui.Build != record.Revision {
		ui.PathStatus = "MISMATCH"
		return ui
	}
	ui.State, ui.Generation, ui.PathStatus = "RUNNING", record.Generation, "MATCHED"
	return ui
}
