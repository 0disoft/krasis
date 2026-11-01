package productcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

type contextAwareWaitClient struct{ apiClient }

type updateWaitReadClient struct {
	apiClient
	runtimeRead func(context.Context) (localapi.RuntimeInfo, error)
}

func (c updateWaitReadClient) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	return c.runtimeRead(ctx)
}

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

func TestUpdateWaitDoesNotCertifyApplicationAfterCancellation(t *testing.T) {
	peer, id, revision := strings.Repeat("b", 64), strings.Repeat("c", 32), strings.Repeat("d", 40)
	client := updateFixture(peer, 119)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.updateFn = func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
		cancel() // A completed read can still arrive after cancellation.
		return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{
			State: "UPDATED", Peer: peer, ID: id, Generation: 120, Revision: revision,
			PeerGeneration: 120, PeerRevision: revision,
		}}, nil
	}
	v, exit, err := waitUpdate(ctx, client, "B", peer, id, 0)
	if err != nil || exit != ExitUnsafeState || v.Code != "canceled" || v.TransferState == "UPDATED" || len(client.requests) != 1 {
		t.Fatalf("late application became success: result=%+v exit=%d err=%v requests=%d", v, exit, err, len(client.requests))
	}
}

func TestUpdateWaitDeadlineFencesEveryReadAtExactBoundary(t *testing.T) {
	peer, id, revision := strings.Repeat("b", 64), strings.Repeat("c", 32), strings.Repeat("d", 40)
	for _, stage := range []string{"initial", "status", "final"} {
		for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			t.Run(stage+"/"+offset.String(), func(t *testing.T) {
				start := time.Now()
				ctx, cancel := context.WithDeadline(context.Background(), start.Add(time.Hour))
				defer cancel()
				deadline, _ := ctx.Deadline()
				now, reads := start, 0
				client := updateFixture(peer, 119)
				client.updateFn = func(localapi.UpdateRequest) (localapi.UpdateResult, error) {
					if stage == "status" {
						now = deadline.Add(offset)
					}
					return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{
						State: "UPDATED", Peer: peer, ID: id, Generation: 120, Revision: revision,
						PeerGeneration: 120, PeerRevision: revision,
					}}, nil
				}
				observed := updateWaitReadClient{client, func(context.Context) (localapi.RuntimeInfo, error) {
					reads++
					if stage == "initial" && reads == 1 || stage == "final" && reads == 2 {
						now = deadline.Add(offset)
					}
					return client.runtime, nil
				}}
				v, exit, err := waitUpdate(ctx, observed, "B", peer, id, time.Second, updateWaitOptions{
					now:   func() time.Time { return now },
					pause: func(context.Context, time.Duration) error { t.Fatal("terminal result polled again"); return nil },
				})
				wantCode, wantExit, wantReads, wantRequests := "updated", ExitOK, 2, 1
				if offset >= 0 {
					wantCode, wantExit = "timeout", ExitUnsafeState
					if stage != "final" {
						wantReads = 1
					}
					if stage == "initial" {
						wantRequests = 0
					}
				}
				if ctx.Err() != nil || err != nil || exit != wantExit || v.Code != wantCode || reads != wantReads || len(client.requests) != wantRequests || offset >= 0 && v.TransferState == "UPDATED" {
					t.Fatalf("result=%+v exit=%d err=%v reads=%d requests=%d", v, exit, err, reads, len(client.requests))
				}
			})
		}
	}
}

func TestUpdateWaitAbortBeforeReadAndDuringFinalRead(t *testing.T) {
	peer, id, revision := strings.Repeat("b", 64), strings.Repeat("c", 32), strings.Repeat("d", 40)
	for _, optional := range []bool{false, true} {
		for _, preAborted := range []bool{false, true} {
			parent, cancelParent := context.WithCancel(context.Background())
			signal, abort := context.WithCancel(context.Background())
			cancel := cancelParent
			opt := updateWaitOptions{}
			if optional {
				opt.signal, cancel = signal, abort
			}
			if preAborted {
				cancel()
			}
			client := updateFixture(peer, 119)
			client.updateFn = func(localapi.UpdateRequest) (localapi.UpdateResult, error) {
				return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{
					State: "UPDATED", Peer: peer, ID: id, Generation: 120, Revision: revision,
					PeerGeneration: 120, PeerRevision: revision,
				}}, nil
			}
			reads := 0
			observed := updateWaitReadClient{client, func(context.Context) (localapi.RuntimeInfo, error) {
				reads++
				if reads == 2 {
					cancel()
				}
				return client.runtime, nil
			}}
			v, exit, err := waitUpdate(parent, observed, "B", peer, id, 0, opt)
			if err != nil || exit != ExitUnsafeState || v.Code != "canceled" || v.TransferState == "UPDATED" || preAborted && (reads != 0 || len(client.requests) != 0) || !preAborted && (reads != 2 || len(client.requests) != 1) || optional && parent.Err() != nil {
				t.Errorf("optional=%v preAborted=%v result=%+v exit=%d err=%v reads=%d requests=%d", optional, preAborted, v, exit, err, reads, len(client.requests))
			}
			cancelParent()
			abort()
		}
	}
}

func TestUpdateWaitInjectedPauseAndReadFailuresNeverReplay(t *testing.T) {
	peer, id := strings.Repeat("b", 64), strings.Repeat("c", 32)
	for _, stage := range []string{"initial", "status", "final", "pause", "deadline"} {
		t.Run(stage, func(t *testing.T) {
			start := time.Now()
			ctx, cancel := context.WithDeadline(context.Background(), start.Add(time.Hour))
			defer cancel()
			now, reads, pauses := start, 0, 0
			fault := errors.New("private I/O failure")
			client := updateFixture(peer, 119)
			client.updateFn = func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
				if r.Action != "STATUS" {
					t.Fatal("observation changed state", r.Action)
				}
				if stage == "status" {
					return localapi.UpdateResult{}, fault
				}
				return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{State: "DELIVERED", Peer: peer, ID: id}}, nil
			}
			observed := updateWaitReadClient{client, func(context.Context) (localapi.RuntimeInfo, error) {
				reads++
				if stage == "initial" && reads == 1 || stage == "final" && reads == 2 {
					return localapi.RuntimeInfo{}, fault
				}
				return client.runtime, nil
			}}
			v, exit, err := waitUpdate(ctx, observed, "B", peer, id, 500*time.Millisecond, updateWaitOptions{
				now: func() time.Time { return now },
				pause: func(context.Context, time.Duration) error {
					pauses++
					if stage == "pause" {
						return fault
					}
					now, _ = ctx.Deadline()
					return nil
				},
			})
			want, requests := "outcome_unknown", 1
			if stage == "deadline" {
				want = "timeout"
			}
			if stage == "initial" {
				requests = 0
			}
			if err != nil || exit != ExitUnsafeState || v.Code != want || len(client.requests) != requests || pauses > 1 {
				t.Fatalf("result=%+v exit=%d err=%v requests=%d pauses=%d", v, exit, err, len(client.requests), pauses)
			}
		})
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
