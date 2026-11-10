//go:build linux

package productcli

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestLinuxDoctorReportsWindowsFeaturesUnsupported(t *testing.T) {
	client := &stubClient{status: localapi.Status{SchemaVersion: localapi.StatusSchemaVersion, State: "CONNECTED"}}
	deps := testDependencies(client)
	deps.localAppData = t.TempDir()
	deps.inspectHost = inspectDoctorHost
	value, code := runDoctor(context.Background(), options{}, deps)
	if code != ExitOK || value.Code != "healthy" || value.Service.State != "UNSUPPORTED" || value.InstalledAgent.State != "UNSUPPORTED" || value.AgentLocation != "USER_SESSION" {
		t.Fatalf("Linux doctor = %+v, exit=%d", value, code)
	}
	for _, action := range value.NextActions {
		if action.Code == "repair_launch_record" || action.Code == "inspect_unavailable_checks" || action.Code == "update_input_installation" {
			t.Fatalf("Linux doctor suggested Windows recovery: %+v", action)
		}
	}
}
