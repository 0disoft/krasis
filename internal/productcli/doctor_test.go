package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func healthyDoctorHost() doctorHost {
	build := localVersion().CLIBuild
	return doctorHost{UI: doctorComponent{State: "RUNNING", Generation: localapi.AgentGeneration, Build: build, PathStatus: "MATCHED"}, LaunchRecord: doctorComponent{State: "VERIFIED", Generation: localapi.AgentGeneration, Build: build}, InstalledAgent: doctorComponent{State: "VERIFIED", Build: build}, Service: doctorComponent{State: "RUNNING", PathStatus: "MATCHED"}, AgentLocation: "PROTECTED"}
}

func TestDoctorDoesNotMistakeConnectedForMatchingInstallation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*doctorHost)
		code   string
		exit   int
	}{
		{"healthy", func(*doctorHost) {}, "healthy", ExitOK},
		{"old-installed-agent", func(h *doctorHost) { h.InstalledAgent.Build = strings.Repeat("b", 40) }, "installed_agent_version_mismatch", ExitUnsafeState},
		{"old-ui", func(h *doctorHost) { h.UI.Generation-- }, "ui_agent_version_mismatch", ExitUnsafeState},
		{"unknown-service", func(h *doctorHost) { h.Service.State = "UNKNOWN" }, "checks_incomplete", ExitUnsafeState},
		{"wrong-service-path", func(h *doctorHost) { h.Service.PathStatus = "MISMATCH" }, "service_path_mismatch", ExitUnsafeState},
		{"stopped-service", func(h *doctorHost) { h.Service.State = "STOPPED" }, "input_service_stopped", ExitUnsafeState},
		{"pending-service", func(h *doctorHost) { h.Service.State = "PENDING" }, "input_service_not_ready", ExitUnsafeState},
		{"invalid-launch-record", func(h *doctorHost) { h.UI.State = "NOT_RUNNING"; h.LaunchRecord.State = "INVALID" }, "launch_record_invalid", ExitUnsafeState},
		{"portable", func(h *doctorHost) {
			h.InstalledAgent.State = "NOT_INSTALLED"
			h.Service = doctorComponent{State: "NOT_INSTALLED"}
			h.AgentLocation = "PORTABLE"
		}, "healthy", ExitOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := healthyDoctorHost()
			test.mutate(&host)
			client := &stubClient{status: localapi.Status{SchemaVersion: localapi.StatusSchemaVersion, State: "CONNECTED"}}
			deps := testDependencies(client)
			deps.inspectHost = func(context.Context, string, string) doctorHost { return host }
			deps.launch = func(string) error { t.Fatal("doctor launched UI"); return nil }
			var out, stderr bytes.Buffer
			exit := run(context.Background(), []string{"doctor", "--json"}, &out, &stderr, deps)
			var result doctorResult
			if json.Unmarshal(out.Bytes(), &result) != nil || exit != test.exit || result.Code != test.code || stderr.Len() != 0 {
				t.Fatalf("exit=%d result=%s stderr=%s", exit, out.String(), stderr.String())
			}
			if len(client.requests) != 0 {
				t.Fatal("doctor changed an update or permission")
			}
			if test.name == "healthy" && result.ProtectedInput != "CONFIGURED" {
				t.Fatal("verified selection omitted")
			}
			if test.exit != ExitOK && result.ProtectedInput == "CONFIGURED" && (test.name == "stopped-service" || test.name == "wrong-service-path" || test.name == "old-installed-agent") {
				t.Fatal("invalid selection claimed configured")
			}
		})
	}
}

func TestDoctorReportsAgentAbsenceAndAuthenticationAsJSON(t *testing.T) {
	for _, test := range []struct {
		err  error
		exit int
		code string
	}{
		{localapi.ErrUnavailable, ExitAgentUnavailable, "agent_unavailable"},
		{errors.Join(localapi.ErrAuthentication, errors.New("private file path")), ExitAuthentication, "local_api_authentication_failed"},
		{localapi.ErrTransport, ExitTransport, "local_api_transport_failed"},
	} {
		deps := dependencies{localAppData: `C:\Users\test\AppData\Local`, openClient: func(string) (apiClient, error) { return nil, test.err }, inspectHost: func(context.Context, string, string) doctorHost {
			h := healthyDoctorHost()
			h.UI.State = "NOT_RUNNING"
			h.AgentLocation = "UNAVAILABLE"
			return h
		}}
		var out, stderr bytes.Buffer
		exit := run(context.Background(), []string{"doctor", "--json"}, &out, &stderr, deps)
		var result doctorResult
		if json.Unmarshal(out.Bytes(), &result) != nil || exit != test.exit || result.Code != test.code || result.Connection != "UNKNOWN" || stderr.Len() != 0 {
			t.Fatalf("exit=%d out=%s stderr=%s", exit, out.String(), stderr.String())
		}
		if strings.Contains(out.String(), "private") {
			t.Fatal("raw native error escaped into output")
		}
	}
}

func TestDoctorFlagsCLIAndRunningAgentBuildMismatch(t *testing.T) {
	client := &stubClient{runtime: localapi.RuntimeInfo{SchemaVersion: localapi.RuntimeSchemaVersion, Protocol: localapi.AgentProtocol, Generation: localapi.AgentGeneration, Build: strings.Repeat("a", 40)}, status: localapi.Status{SchemaVersion: localapi.StatusSchemaVersion, State: "CONNECTED"}}
	var out, stderr bytes.Buffer
	if run(context.Background(), []string{"doctor", "--json"}, &out, &stderr, testDependencies(client)) != ExitUnsafeState || !strings.Contains(out.String(), "cli_agent_version_mismatch") {
		t.Fatal("same generation hid different source builds")
	}
}
