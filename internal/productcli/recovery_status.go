package productcli

import (
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/productlog"
)

type recoveryStatusResult struct {
	Schema  string                      `json:"schemaVersion"`
	Code    string                      `json:"code"`
	Task    string                      `json:"task"`
	LastRun *productlog.RecoverySummary `json:"lastRun,omitempty"`
}

func recoveryStatus(base string) (recoveryStatusResult, int) {
	v := recoveryStatusResult{Schema: "krasis.cli.recovery-status/v1", Code: "not_recorded", Task: "UNKNOWN"}
	receipt, err := productlog.RecoveryReceipt(base)
	if err != nil {
		v.Code = "receipt_unavailable"
		return v, ExitUnsafeState
	}
	v.LastRun = receipt
	if receipt != nil {
		v.Code = "recorded"
	}
	active, err := appsettings.RecoveryTaskEnabled(base)
	if err == nil {
		if active {
			v.Task = "ENABLED"
		} else {
			v.Task = "DISABLED"
		}
	}
	return v, ExitOK
}
