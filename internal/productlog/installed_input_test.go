package productlog

import (
	"testing"

	"github.com/0disoft/krasis/internal/inputworker"
)

func TestInputConnectionDiagnosticAllowlistRejectsDynamicStages(t *testing.T) {
	if !allowed[inputworker.ConnectionFailureEvent(nil)] || !allowed[inputworker.ConnectionNativeFailureEvent(nil)] {
		t.Fatal("unknown input connection classification would be dropped")
	}
	for _, code := range []string{"installed_input_stage_pipe_open", "installed_input_stage_service_config", "installed_input_stage_worker_ready", "installed_input_native_5"} {
		if !allowed[code] {
			t.Fatal("fixed input connection diagnostic omitted")
		}
	}
	if allowed["installed_input_stage_private-path"] || allowed["installed_input_native_123456"] {
		t.Fatal("unbounded input connection diagnostic accepted")
	}
}
