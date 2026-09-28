package productcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestVersionWorksWithoutAgentAndReportsRunningGeneration(t *testing.T) {
	for _, test := range []struct {
		name string
		deps dependencies
		want string
	}{
		{"offline", dependencies{localAppData: `C:\Users\test\AppData\Local`, openClient: func(string) (apiClient, error) { return nil, localapi.ErrUnavailable }}, `"agentState":"UNAVAILABLE"`},
		{"running", testDependencies(&stubClient{}), `"agentGeneration":`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			if code := run(context.Background(), []string{"version", "--json"}, &out, &errs, test.deps); code != ExitOK || !strings.Contains(out.String(), test.want) || errs.Len() != 0 {
				t.Fatalf("version code=%d out=%q err=%q", code, out.String(), errs.String())
			}
		})
	}
}

func TestDoctorSeparatesDisconnectedAndProtocolMismatch(t *testing.T) {
	for _, test := range []struct {
		state    string
		protocol int
		want     int
	}{
		{"CONNECTED", localapi.AgentProtocol, ExitOK},
		{"LOCAL", localapi.AgentProtocol, ExitUnsafeState},
		{"CONNECTED", localapi.AgentProtocol + 1, ExitTransport},
	} {
		client := &stubClient{status: localapi.Status{SchemaVersion: localapi.StatusSchemaVersion, State: test.state}, runtime: localapi.RuntimeInfo{SchemaVersion: localapi.RuntimeSchemaVersion, Protocol: test.protocol, Generation: localapi.AgentGeneration, Build: "development", Instance: strings.Repeat("a", 32)}}
		var out, errs bytes.Buffer
		if code := run(context.Background(), []string{"doctor", "--json"}, &out, &errs, testDependencies(client)); code != test.want || !strings.Contains(out.String(), `"schemaVersion":"krasis.cli.doctor/v1"`) || errs.Len() != 0 {
			t.Fatalf("doctor state=%s protocol=%d code=%d out=%q err=%q", test.state, test.protocol, code, out.String(), errs.String())
		}
	}
}
