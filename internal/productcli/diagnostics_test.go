package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestDiagnosticSendQueuesOnceWithoutLeakingAgentPath(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 85)
	calls := 0
	client.diagnosticFn = func(got string) (localapi.DiagnosticTransferResult, error) {
		calls++
		if got != peer {
			t.Fatalf("wrong peer: %q", got)
		}
		return localapi.DiagnosticTransferResult{Code: "queued", Path: `C:\Users\secret\support-latest.json`}, nil
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"diagnostics", "send", "B", "--json"}, &stdout, &stderr, testDependencies(client))
	var result diagnosticTransferResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != ExitOK || result.Code != "queued" || result.Transfer != "QUEUED" || result.PeerAlias != "B" || calls != 1 || stderr.Len() != 0 || strings.Contains(stdout.String(), "secret") {
		t.Fatalf("code=%d result=%+v calls=%d stdout=%q stderr=%q", code, result, calls, stdout.String(), stderr.String())
	}
}

func TestDiagnosticSendRejectsUnknownOrBusyPeerWithoutQueueing(t *testing.T) {
	peer := strings.Repeat("b", 64)
	for _, test := range []struct {
		name, want string
		prepare    func(*stubClient)
	}{
		{"unknown", "peer_not_found", func(c *stubClient) { c.trust.Peers = nil }},
		{"busy", "already_queued", func(c *stubClient) {
			c.updateFn = func(localapi.UpdateRequest) (localapi.UpdateResult, error) {
				return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Status: peerupdate.Status{Diagnostic: "QUEUED"}}, nil
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := updateFixture(peer, 85)
			test.prepare(client)
			client.diagnosticFn = func(string) (localapi.DiagnosticTransferResult, error) {
				t.Fatal("unsafe diagnostic send")
				return localapi.DiagnosticTransferResult{}, nil
			}
			var out bytes.Buffer
			code := run(context.Background(), []string{"diagnostics", "send", "B", "--json"}, &out, &out, testDependencies(client))
			if code != ExitUnsafeState || !strings.Contains(out.String(), `"code":"`+test.want+`"`) {
				t.Fatalf("code=%d output=%q", code, out.String())
			}
		})
	}
}

func TestDiagnosticSendUncertainResponseIsNotRetried(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 85)
	calls := 0
	client.diagnosticFn = func(string) (localapi.DiagnosticTransferResult, error) {
		calls++
		return localapi.DiagnosticTransferResult{}, errors.New("private transport detail")
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"diagnostics", "send", "B", "--json"}, &stdout, &stderr, testDependencies(client)); code != ExitUnsafeState || calls != 1 || !strings.Contains(stdout.String(), `"code":"outcome_unknown"`) || strings.Contains(stdout.String(), "private") || stderr.Len() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
}

func TestDiagnosticStatusIsGlobalAndReadOnly(t *testing.T) {
	client := &stubClient{updateFn: func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
		if r.Action != "STATUS" {
			t.Fatalf("unexpected action: %s", r.Action)
		}
		return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Status: peerupdate.Status{Diagnostic: "SENT"}}, nil
	}}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"diagnostics", "status", "--json"}, &stdout, &stderr, testDependencies(client)); code != ExitOK || !strings.Contains(stdout.String(), `"transfer":"SENT"`) || strings.Contains(stdout.String(), "peerAlias") || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
