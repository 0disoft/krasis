package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/logindiag"
)

func TestInputServiceStagesSurviveSupportWithoutPrivateCause(t *testing.T) {
	for _, operation := range []string{
		"input-startup-validation", "input-startup-prepare", "input-startup-authentication", "input-startup-transfer", "input-startup-ready", "input-startup-publish",
		"input-prepare-launch", "input-prepare-connect", "input-prepare-permit",
		"input-retire-validation", "input-retire-deadline", "input-retire-authentication", "input-retire-stop", "input-retire-receipt",
		"input-close-control", "input-close-worker", "input-retire-SECRET/path",
	} {
		t.Run(operation, func(t *testing.T) {
			raw := []byte(`{"schema":"krasis.support/v1","created":"2026-10-23T08:02:00Z","logs":[],"loginRuntime":{"startedAt":"2026-10-23T08:00:00Z","time":"2026-10-23T08:01:00Z","service":"FAILED","console":"USER_SESSION","grant":"PRESENT","lastWorker":"NONE","lastFailure":{"time":"2026-10-23T08:01:00Z","stage":"service","reason":"input","operation":"` + operation + `","win32Error":109,"win32Known":true,"workerExit":{"state":"EXITED","code":3221225477,"known":true},"rawCause":"SECRET/path"}}}`)
			clean, err := SanitizeSupport(raw)
			if err != nil || strings.Contains(string(clean), "SECRET") || strings.Contains(string(clean), "rawCause") {
				t.Fatal("private cause leaked", err)
			}
			var got struct {
				Runtime *logindiag.Runtime `json:"loginRuntime"`
			}
			if err := json.Unmarshal(clean, &got); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(operation, "SECRET") {
				if got.Runtime != nil || !strings.Contains(string(clean), "loginRuntimeIncomplete") {
					t.Fatal("private operation accepted")
				}
				return
			}
			if got.Runtime == nil || !got.Runtime.Valid() || got.Runtime.LastFailure == nil {
				t.Fatal("service stage discarded")
			}
			f := got.Runtime.LastFailure
			if f.Operation != operation || f.Win32Error != 109 || !f.Win32Known || f.WorkerExit == nil || f.WorkerExit.Code != 0xc0000005 {
				t.Fatal("service cause or exit disappeared", f)
			}
		})
	}
}
