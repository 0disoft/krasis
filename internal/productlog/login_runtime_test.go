package productlog

import (
	"encoding/json"
	"github.com/0disoft/krasis/internal/logindiag"
	"strings"
	"testing"
)

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
	v.Console = "SECRET/user"
	data, _ = json.Marshal(v)
	clean, err = SanitizeSupport(envelope(data))
	if err != nil || strings.Contains(string(clean), "SECRET") || !strings.Contains(string(clean), "loginRuntimeIncomplete") {
		t.Fatal("untrusted console detail escaped")
	}
}
