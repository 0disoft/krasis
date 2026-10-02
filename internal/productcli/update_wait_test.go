package productcli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

type contextAwareWaitClient struct{ apiClient }

func (c contextAwareWaitClient) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	if ctx.Err() != nil {
		return localapi.RuntimeInfo{}, ctx.Err()
	}
	return c.apiClient.Runtime(ctx)
}
func (c contextAwareWaitClient) Trust(ctx context.Context) (localapi.Trust, error) {
	if ctx.Err() != nil {
		return localapi.Trust{}, ctx.Err()
	}
	return c.apiClient.Trust(ctx)
}

func TestUpdateWaitInitialReadHonorsCancellationAndDeadline(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "timeout"}[deadline], func(t *testing.T) {
			client := updateFixture(peer, 119)
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			} else {
				cancel()
			}
			defer cancel()
			aware := contextAwareWaitClient{client}
			v, exit, err := waitUpdate(ctx, aware, "B", peer, id, 0)
			want := map[bool]string{false: "canceled", true: "timeout"}[deadline]
			if err != nil || exit != ExitUnsafeState || v.Code != want || len(client.requests) != 0 {
				t.Fatalf("initial runtime: %+v exit=%d err=%v", v, exit, err)
			}
			value, exit, err := dispatchUpdateWait(ctx, aware, []string{"wait", "B", id})
			result, ok := value.(updateWaitResult)
			if err != nil || exit != ExitUnsafeState || !ok || result.Code != want || len(client.requests) != 0 {
				t.Fatalf("initial trust: value=%+v exit=%d err=%v", value, exit, err)
			}
		})
	}
}

func TestUpdateWaitRequiresExactApplication(t *testing.T) {
	peer, id, revision := strings.Repeat("b", 64), strings.Repeat("c", 32), strings.Repeat("d", 40)
	for _, test := range []struct {
		final, code string
		exit        int
	}{
		{"UPDATED", "updated", ExitOK}, {"PEER_APPLY_FAILED", "update_failed", ExitUnsafeState},
		{"HASH_MISMATCH", "update_failed", ExitUnsafeState}, {"REPLACED", "transfer_replaced", ExitUnsafeState},
		{"MISMATCH", "outcome_unknown", ExitUnsafeState}, {"AGENT_CHANGED", "agent_changed", ExitUnsafeState},
	} {
		t.Run(test.final, func(t *testing.T) {
			client := updateFixture(peer, 119)
			client.runtime = localapi.RuntimeInfo{SchemaVersion: localapi.RuntimeSchemaVersion, Instance: strings.Repeat("a", 32)}
			polls := 0
			client.updateFn = func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
				if r.Action != "STATUS" {
					t.Fatalf("wait changed state: %s", r.Action)
				}
				states := []string{"QUEUED", "SENDING", "DELIVERED", "TRANSFER_UNKNOWN", "RESTART_UNCONFIRMED", test.final}
				s := peerupdate.Status{State: states[polls], Peer: peer, ID: id, Generation: 120, Revision: revision, PeerGeneration: 120, PeerRevision: revision}
				polls++
				switch s.State {
				case "REPLACED":
					s.ID = strings.Repeat("a", 32)
				case "MISMATCH":
					s.State = "UPDATED"
					s.PeerRevision = strings.Repeat("a", 40)
				case "AGENT_CHANGED":
					client.runtime.Instance = strings.Repeat("e", 32)
				}
				return localapi.UpdateResult{Code: "ok", Status: s}, nil
			}
			v, exit, err := waitUpdate(context.Background(), client, "B", peer, id, 0)
			if err != nil || exit != test.exit || v.Code != test.code || polls != 6 {
				t.Fatalf("result=%+v exit=%d polls=%d err=%v", v, exit, polls, err)
			}
		})
	}
}

func TestUpdateWaitTimeoutNeverResends(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	client := updateFixture(peer, 119)
	ctx, cancel := context.WithCancel(context.Background())
	client.updateFn = func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
		cancel()
		return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{State: "DELIVERED", Peer: peer, ID: id}}, nil
	}
	v, exit, err := waitUpdate(ctx, client, "B", peer, id, time.Hour)
	if err != nil || exit != ExitUnsafeState || v.Code != "canceled" || len(client.requests) != 1 {
		t.Fatalf("result=%+v exit=%d err=%v", v, exit, err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	v, exit, _ = waitUpdate(ctx, client, "B", peer, id, 0)
	if exit != ExitUnsafeState || v.Code != "timeout" {
		t.Fatalf("result=%+v", v)
	}
}

func TestUpdateWaitArgumentValidationBeforeOpeningAgent(t *testing.T) {
	for _, args := range [][]string{
		{"update", "wait", "B"}, {"update", "wait", "B", "bad"},
		{"update", "wait", "B", strings.Repeat("c", 32), "--timeout", "0s"},
		{"update", "wait", "B", strings.Repeat("c", 32), "--timeout", "11m"},
		{"update", "wait", "B", strings.Repeat("c", 32), "--timeout"},
		{"update", "wait", "B", strings.Repeat("c", 32), "--timeout", "--json", "1s"},
		{"update", "wait", "B", strings.Repeat("c", 32), "--timeout", "1s", "--timeout", "2s"},
		{"status", "--timeout", "1s"},
	} {
		var out, errs bytes.Buffer
		d := dependencies{openClient: func(string) (apiClient, error) { t.Fatal("opened agent for invalid arguments"); return nil, nil }}
		if code := run(context.Background(), args, &out, &errs, d); code != ExitUsage {
			t.Fatalf("args=%v code=%d", args, code)
		}
	}
}

func TestUpdateWaitTimeoutThroughCLI(t *testing.T) {
	peer, id, revision := strings.Repeat("b", 64), strings.Repeat("c", 32), strings.Repeat("d", 40)
	for _, args := range [][]string{
		{"update", "wait", "B", id, "--timeout", "1s", "--json"},
		{"--json", "update", "wait", "B", id, "--timeout", "10m"},
		{"update", "wait", "B", id, "--json", "--timeout", "2m"},
	} {
		client := updateFixture(peer, 119)
		client.updateFn = func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
			if r.Action != "STATUS" {
				t.Fatalf("wait changed state: %s", r.Action)
			}
			return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{
				State: "UPDATED", Peer: peer, ID: id, Generation: 120, Revision: revision,
				PeerGeneration: 120, PeerRevision: revision,
			}}, nil
		}
		var out, errs bytes.Buffer
		code := run(context.Background(), args, &out, &errs, testDependencies(client))
		if code != ExitOK || errs.Len() != 0 || !strings.Contains(out.String(), `"code":"updated"`) || len(client.requests) != 1 {
			t.Fatalf("args=%v code=%d output=%q errors=%q requests=%+v", args, code, out.String(), errs.String(), client.requests)
		}
	}
}
