package productlog

import (
	"encoding/json"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
	"strings"
	"testing"
)

func TestThreadRetirementDetailsSurviveSupportWithoutThreadIdentity(t *testing.T) {
	for _, operation := range inputdesktop.CaptureThreadFailureOperations() {
		raw := []byte(`{"schema":"krasis.support/v1","created":"2026-10-15T08:02:00Z","logs":[],"loginRuntime":{"startedAt":"2026-10-15T08:00:00Z","time":"2026-10-15T08:01:00Z","service":"FAILED","console":"USER_SESSION","grant":"PRESENT","lastWorker":"NONE","lastFailure":{"time":"2026-10-15T08:01:00Z","stage":"service","reason":"capture","operation":"` + operation + `","win32Error":1460,"win32Known":true,"threadId":"SECRET"}}}`)
		clean, err := SanitizeSupport(raw)
		if err != nil || !strings.Contains(string(clean), operation) || strings.Contains(string(clean), "SECRET") || strings.Contains(string(clean), "threadId") {
			t.Fatal("bounded native thread detail lost or private field leaked", operation, err)
		}
	}
}

func TestRuntimeReceiptTransferRetainsFailureWithoutPrivateFields(t *testing.T) {
	v := logindiag.Runtime{StartedAt: "2026-10-11T00:00:00Z", Time: "2026-10-11T00:01:00Z", Service: "READY", Console: "USER_SESSION", Grant: "PRESENT", LastWorker: "FAILED", LastFailure: &logindiag.RuntimeFailure{Time: "2026-10-11T00:00:30Z", Stage: "start", Win32Error: 5, Win32Known: true}}
	data, _ := json.Marshal(v)
	envelope := func(data []byte) []byte {
		return []byte(`{"schema":"krasis.support/v1","created":"2026-10-11T00:02:00Z","logs":[],"loginRuntime":` + string(data) + `}`)
	}
	clean, err := SanitizeSupport(envelope(data))
	if err != nil || !strings.Contains(string(clean), `"lastWorker":"FAILED"`) || !strings.Contains(string(clean), `"win32Error":5`) {
		t.Fatal("runtime evidence lost", err)
	}
	v.LastFailure.Stage, v.LastFailure.Reason = "worker", "desktop"
	data, _ = json.Marshal(v)
	clean, err = SanitizeSupport(envelope(data))
	if err != nil || !strings.Contains(string(clean), `"reason":"desktop"`) {
		t.Fatal("terminal reason lost", err)
	}
	v.LastFailure.Reason = "SECRET/window/input"
	data, _ = json.Marshal(v)
	clean, err = SanitizeSupport(envelope(data))
	if err != nil || strings.Contains(string(clean), "SECRET") || !strings.Contains(string(clean), "loginRuntimeIncomplete") {
		t.Fatal("private reason escaped", err)
	}
	v.LastFailure.Reason = "desktop"
	v.Console = "SECRET/user"
	data, _ = json.Marshal(v)
	clean, err = SanitizeSupport(envelope(data))
	if err != nil || strings.Contains(string(clean), "SECRET") || !strings.Contains(string(clean), "loginRuntimeIncomplete") {
		t.Fatal("untrusted console detail escaped")
	}
}
