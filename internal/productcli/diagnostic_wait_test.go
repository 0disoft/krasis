package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestDiagnosticWaitObservesOnlyRequestedCollection(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	for _, test := range []struct {
		name, code string
		exit       int
	}{
		{"received", "saved", ExitOK},
		{"expired", "not_received", ExitUnsafeState},
		{"failed", "not_received", ExitUnsafeState},
		{"missing_file", "outcome_unknown", ExitUnsafeState},
		{"different_id", "request_changed", ExitUnsafeState},
		{"agent_changed", "agent_changed", ExitUnsafeState},
		{"lost_reply", "outcome_unknown", ExitUnsafeState},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := updateFixture(peer, 118)
			client.runtime = localapi.RuntimeInfo{SchemaVersion: localapi.RuntimeSchemaVersion, Instance: strings.Repeat("a", 32)}
			polls := 0
			client.collectionFn = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
				if r.Action != "STATUS" || r.Peer != peer || r.ID != id {
					t.Fatalf("wait started or followed another request: %+v", r)
				}
				polls++
				result := localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "PENDING", LocalPath: "A.json"}
				if polls == 1 {
					return result, nil
				}
				result.State, result.PeerPath = "RECEIVED", "B.json"
				switch test.name {
				case "expired":
					result.State, result.PeerPath = "EXPIRED", ""
				case "failed":
					result.State, result.PeerPath = "FAILED", ""
				case "missing_file":
					result.PeerPath = ""
				case "different_id":
					result.ID = strings.Repeat("d", 32)
				case "agent_changed":
					client.runtime.Instance = strings.Repeat("e", 32)
				case "lost_reply":
					return localapi.DiagnosticCollectionResult{}, errors.New("private transport detail")
				}
				return result, nil
			}
			v := diagnosticCollectionResult{SchemaVersion: diagnosticCollectionSchema, PeerAlias: "B", ID: id, Code: "outcome_unknown"}
			result, exit, err := waitDiagnostic(context.Background(), client, v, peer, 0)
			if err != nil || exit != test.exit || result.Code != test.code || polls != 2 {
				t.Fatalf("result=%+v exit=%d polls=%d err=%v", result, exit, polls, err)
			}
			if (test.name == "different_id" || test.name == "agent_changed") && (result.State != "" || result.LocalPath != "" || result.PeerPath != "") {
				t.Fatalf("foreign result leaked paths: %+v", result)
			}
		})
	}
}

func TestDiagnosticWaitCancellationAndDeadlineNeverStartWork(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	for _, deadline := range []bool{false, true} {
		client := updateFixture(peer, 118)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		want := "canceled"
		if deadline {
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = "timeout"
		}
		client.collectionFn = func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
			t.Fatal("canceled wait performed a collection request")
			return localapi.DiagnosticCollectionResult{}, nil
		}
		value, exit, err := dispatchDiagnosticWait(ctx, contextAwareWaitClient{client}, []string{"wait", "B", id})
		cancel()
		result, ok := value.(diagnosticCollectionResult)
		if err != nil || !ok || exit != ExitUnsafeState || result.Code != want {
			t.Fatalf("initial lookup: result=%+v exit=%d err=%v", value, exit, err)
		}
	}
	client := updateFixture(peer, 118)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	client.collectionFn = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Action != "STATUS" {
			t.Fatal("canceled wait started another export")
		}
		requests++
		cancel()
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "PENDING"}, nil
	}
	v := diagnosticCollectionResult{SchemaVersion: diagnosticCollectionSchema, PeerAlias: "B", ID: id, Code: "outcome_unknown"}
	result, exit, err := waitDiagnostic(ctx, client, v, peer, time.Hour)
	if err != nil || exit != ExitUnsafeState || result.Code != "canceled" || requests != 1 {
		t.Fatalf("poll canceled: result=%+v exit=%d requests=%d err=%v", result, exit, requests, err)
	}
}

func TestDiagnosticWaitThroughCLIAndArgumentValidation(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	client := updateFixture(peer, 118)
	client.collectionFn = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Action != "STATUS" || r.Peer != peer || r.ID != id {
			t.Fatalf("wait changed state: %+v", r)
		}
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "RECEIVED", LocalPath: "A.json", PeerPath: "B.json"}, nil
	}
	for _, args := range [][]string{
		{"diagnostics", "wait", "B", id, "--json"},
		{"--json", "diagnostics", "wait", "B", id, "--timeout", "1s"},
	} {
		var out, errs bytes.Buffer
		code := run(context.Background(), args, &out, &errs, testDependencies(client))
		var result diagnosticCollectionResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if code != ExitOK || result.Code != "saved" || result.LocalPath != "A.json" || result.PeerPath != "B.json" || errs.Len() != 0 {
			t.Fatalf("args=%v code=%d result=%+v errs=%q", args, code, result, errs.String())
		}
	}
	for _, args := range [][]string{
		{"diagnostics", "wait", "B"}, {"diagnostics", "wait", "B", "bad"},
		{"diagnostics", "wait", "B", id, "--timeout", "0s"},
		{"diagnostics", "wait", "B", id, "--timeout", "11m"},
		{"diagnostics", "wait", "B", id, "--timeout", "1s", "--timeout", "2s"},
	} {
		var out, errs bytes.Buffer
		deps := dependencies{openClient: func(string) (apiClient, error) { t.Fatal("invalid arguments opened agent"); return nil, nil }}
		if code := run(context.Background(), args, &out, &errs, deps); code != ExitUsage {
			t.Fatalf("args=%v code=%d", args, code)
		}
	}
}

type diagnosticWaitRuntimeClient struct {
	apiClient
	read func(context.Context) (localapi.RuntimeInfo, error)
}

func (c diagnosticWaitRuntimeClient) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	return c.read(ctx)
}

func TestDiagnosticWaitCannotConfirmFilesAfterRuntimeReadFailure(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	base := updateFixture(peer, 118)
	requests := 0
	base.collectionFn = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Action != "STATUS" {
			t.Fatal("runtime failure restarted collection")
		}
		requests++
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "RECEIVED", LocalPath: "A.json", PeerPath: "B.json"}, nil
	}
	reads := 0
	client := diagnosticWaitRuntimeClient{apiClient: base, read: func(ctx context.Context) (localapi.RuntimeInfo, error) {
		reads++
		if reads == 2 {
			return localapi.RuntimeInfo{}, errors.New("private runtime detail")
		}
		return base.Runtime(ctx)
	}}
	v := diagnosticCollectionResult{SchemaVersion: diagnosticCollectionSchema, PeerAlias: "B", ID: id, Code: "outcome_unknown"}
	result, exit, err := waitDiagnostic(context.Background(), client, v, peer, 0)
	if err != nil || exit != ExitUnsafeState || result.Code != "outcome_unknown" || result.LocalPath != "" || result.PeerPath != "" || requests != 1 {
		t.Fatalf("result=%+v exit=%d requests=%d err=%v", result, exit, requests, err)
	}
}

func TestDiagnosticWaitDeadlineInterruptsPendingPoll(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	client := updateFixture(peer, 118)
	requests := 0
	client.collectionFn = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Action != "STATUS" {
			t.Fatal("deadline restarted collection")
		}
		requests++
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "PENDING"}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	v := diagnosticCollectionResult{SchemaVersion: diagnosticCollectionSchema, PeerAlias: "B", ID: id, Code: "outcome_unknown"}
	result, exit, err := waitDiagnostic(ctx, client, v, peer, time.Hour)
	if err != nil || exit != ExitUnsafeState || result.Code != "timeout" || requests > 1 {
		t.Fatalf("result=%+v exit=%d requests=%d err=%v", result, exit, requests, err)
	}
}
