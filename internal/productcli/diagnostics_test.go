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

func TestDiagnosticCollectReportsBothSavedFiles(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 95)
	id := strings.Repeat("c", 32)
	starts, statuses := 0, 0
	client.collectionFn = func(request localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if request.Peer != peer {
			t.Fatal("wrong peer")
		}
		if request.Action == "START" {
			starts++
			return localapi.DiagnosticCollectionResult{Code: "queued", ID: id, LocalPath: "A.json"}, nil
		}
		if request.Action == "STATUS" && request.ID == id {
			statuses++
			return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "RECEIVED", LocalPath: "A.json", PeerPath: "B.json"}, nil
		}
		t.Fatal("unexpected collection request")
		return localapi.DiagnosticCollectionResult{}, nil
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"diagnostics", "collect", "B", "--json"}, &stdout, &stderr, testDependencies(client))
	var result diagnosticCollectionResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != ExitOK || result.Code != "saved" || result.LocalPath != "A.json" || result.PeerPath != "B.json" || starts != 1 || statuses != 1 || stderr.Len() != 0 {
		t.Fatalf("code=%d result=%+v starts=%d statuses=%d stderr=%q", code, result, starts, statuses, stderr.String())
	}
}

func TestDiagnosticCollectUncertainStartNeverRetries(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 95)
	calls := 0
	client.collectionFn = func(request localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		calls++
		return localapi.DiagnosticCollectionResult{}, errors.New("private path")
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"diagnostics", "collect", "B", "--json"}, &stdout, &stderr, testDependencies(client))
	if code != ExitUnsafeState || calls != 1 || !strings.Contains(stdout.String(), `"code":"outcome_unknown"`) || strings.Contains(stdout.String(), "private path") || stderr.Len() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
}

func TestDiagnosticCollectStorageLimitExplainsFailureWithoutRetrying(t *testing.T) {
	peer := strings.Repeat("b", 64)
	for _, asJSON := range []bool{true, false} {
		client := updateFixture(peer, 202)
		calls := 0
		client.collectionFn = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
			calls++
			if r.Action != "START" {
				t.Fatal("failed collection was polled")
			}
			return localapi.DiagnosticCollectionResult{Code: "storage_limit"}, nil
		}
		args := []string{"diagnostics", "collect", "B"}
		if asJSON {
			args = append(args, "--json")
		}
		var out, errs bytes.Buffer
		code := run(context.Background(), args, &out, &errs, testDependencies(client))
		if code != ExitUnsafeState || calls != 1 || errs.Len() != 0 || !strings.Contains(out.String(), "진단 보관 한도") {
			t.Fatal(code, calls, out.String(), errs.String())
		}
		if asJSON {
			var result diagnosticCollectionResult
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Code != "storage_limit" || result.ID != "" || result.LocalPath != "" || result.PeerPath != "" {
				t.Fatal(result, err)
			}
		}
	}
}

func TestDiagnosticCollectStatusDoesNotStartAnotherExport(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 95)
	id := strings.Repeat("c", 32)
	client.collectionFn = func(request localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if request.Action != "STATUS" || request.Peer != peer || request.ID != id {
			t.Fatal("status started work")
		}
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "PENDING", LocalPath: "A.json"}, nil
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"diagnostics", "collect-status", "B", id, "--json"}, &stdout, &stderr, testDependencies(client))
	if code != ExitUnsafeState || !strings.Contains(stdout.String(), `"code":"pending"`) || !strings.Contains(stdout.String(), `"localPath":"A.json"`) || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
