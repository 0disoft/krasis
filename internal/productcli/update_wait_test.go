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
	} {
		var out, errs bytes.Buffer
		d := dependencies{openClient: func(string) (apiClient, error) { t.Fatal("opened agent for invalid arguments"); return nil, nil }}
		if code := run(context.Background(), args, &out, &errs, d); code != ExitUsage {
			t.Fatalf("args=%v code=%d", args, code)
		}
	}
}
