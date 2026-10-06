package productcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type appOpenClient struct {
	stubClient
	calls  int
	openFn func(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error)
}

func (c *appOpenClient) OpenApp(ctx context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
	c.calls++
	if c.openFn != nil {
		return c.openFn(ctx, r)
	}
	return localapi.AppOpenResult{Code: "launch_requested", ID: r.ID}, nil
}

func TestCLIAppOpenStatusAndUnknownPeer(t *testing.T) {
	client := &appOpenClient{stubClient: stubClient{trust: localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion, Peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("a", 64)}}}}}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"app", "open-status", "B", strings.Repeat("b", 32), "--json"}, &out, &errOut, testDependencies(client)); code != ExitUnsafeState || !strings.Contains(out.String(), "launch_requested") || client.calls != 1 {
		t.Fatalf("code=%d out=%s", code, out.String())
	}
	if code := run(context.Background(), []string{"app", "open", "unknown", "--json"}, &out, &errOut, testDependencies(client)); code != ExitUnsafeState || client.calls != 1 {
		t.Fatal("unknown peer called API")
	}
	if code := run(context.Background(), []string{"app", "open", "B", "other.exe"}, &out, &errOut, testDependencies(client)); code != ExitUsage || client.calls != 1 {
		t.Fatal("path argument accepted")
	}
}

func scriptedAppOpenClient(fn func(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error)) *appOpenClient {
	return &appOpenClient{
		stubClient: stubClient{trust: localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion,
			Peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("a", 64)}}}},
		openFn: fn,
	}
}

func TestCLIAppOpenWaitsForWindowReadyWithOneRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		id := strings.Repeat("b", 32)
		var requests []localapi.AppOpenRequest
		states := []string{"pending", "launch_accepted", "launch_requested", "window_ready"}
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			requests = append(requests, r)
			if r.Action == "START" {
				return localapi.AppOpenResult{Code: "queued", ID: id}, nil
			}
			code := states[0]
			states = states[1:]
			return localapi.AppOpenResult{Code: code, ID: id}, nil
		})
		var out, errOut bytes.Buffer
		if exit := run(context.Background(), []string{"app", "open", "B", "--json"}, &out, &errOut, testDependencies(client)); exit != ExitOK || !strings.Contains(out.String(), "window_ready") || errOut.Len() != 0 {
			t.Fatal("returned before window readiness", exit, out.String(), errOut.String())
		}
		if len(requests) != 5 || requests[0].Action != "START" || requests[0].ID != "" {
			t.Fatal("incorrect launch request count", requests)
		}
		for _, r := range requests[1:] {
			if r.Action != "STATUS" || r.ID != id || r.Peer != requests[0].Peer {
				t.Fatal("polling changed the peer or request", r)
			}
		}
	})
}

func TestCLIAppOpenTimeoutRetainsRequestAndUnconfirmedOutcome(t *testing.T) {
	for _, state := range []string{"pending", "launch_accepted", "launch_requested"} {
		t.Run(state, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				id := strings.Repeat("b", 32)
				starts := 0
				client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
					if r.Action == "START" {
						starts++
						return localapi.AppOpenResult{Code: "queued", ID: id}, nil
					}
					return localapi.AppOpenResult{Code: state, ID: id}, nil
				})
				started := time.Now()
				value, exit, err := openPeerApp(context.Background(), client, []string{"open", "B"})
				result := value.(appOpenResult)
				want := "window_unconfirmed"
				if state == "pending" {
					want = "outcome_unknown"
				}
				if err != nil || exit != ExitUnsafeState || result.Code != want || result.ID != id || starts != 1 || time.Since(started) != 70*time.Second {
					t.Fatal("wait was unbounded, repeated launch or reported success", result, exit, err, starts)
				}
			})
		})
	}
}

func TestCLIAppOpenRejectsLostAndMismatchedReceipts(t *testing.T) {
	for _, failure := range []string{"transport", "wrong_id", "unknown_code", "failed"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				id := strings.Repeat("b", 32)
				starts := 0
				client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
					if r.Action == "START" {
						starts++
						return localapi.AppOpenResult{Code: "queued", ID: id}, nil
					}
					switch failure {
					case "transport":
						return localapi.AppOpenResult{}, errors.New("private transport error")
					case "wrong_id":
						return localapi.AppOpenResult{Code: "window_ready", ID: strings.Repeat("c", 32)}, nil
					case "unknown_code":
						return localapi.AppOpenResult{Code: "private server detail", ID: id}, nil
					default:
						return localapi.AppOpenResult{Code: "launch_failed", ID: id}, nil
					}
				})
				value, exit, err := openPeerApp(context.Background(), client, []string{"open", "B"})
				result := value.(appOpenResult)
				want := "outcome_unknown"
				if failure == "failed" {
					want = "launch_failed"
				}
				if err != nil || exit != ExitUnsafeState || result.Code != want || result.ID != id || starts != 1 {
					t.Fatal("uncertain receipt caused success or a new launch", result, exit, err, starts)
				}
			})
		})
	}
}

func TestCLIAppOpenCancellationCannotAcceptLateWindowReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		id := strings.Repeat("b", 32)
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			if r.Action == "START" {
				return localapi.AppOpenResult{Code: "queued", ID: id}, nil
			}
			cancel()
			return localapi.AppOpenResult{Code: "window_ready", ID: id}, nil
		})
		value, exit, err := openPeerApp(ctx, client, []string{"open", "B"})
		result := value.(appOpenResult)
		if err != nil || exit != ExitUnsafeState || result.Code != "canceled" || result.ID != id || client.calls != 2 {
			t.Fatal("canceled wait accepted a late response", result, exit, err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
		cancel()
		return localapi.AppOpenResult{}, context.Canceled
	})
	value, exit, err := openPeerApp(ctx, client, []string{"open", "B"})
	if err != nil || exit != ExitUnsafeState || value.(appOpenResult).Code != "canceled" || client.calls != 1 {
		t.Fatal("canceled launch request was retried or reported success", value, exit, err)
	}
}

func TestCLIAppOpenStatusSucceedsOnlyForMatchingReadyReceipt(t *testing.T) {
	for _, state := range []string{"pending", "launch_accepted", "launch_requested", "window_ready", "window_unconfirmed"} {
		t.Run(state, func(t *testing.T) {
			id := strings.Repeat("b", 32)
			client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
				if r.Action != "STATUS" || r.ID != id {
					t.Fatal("status query sent a launch request", r)
				}
				return localapi.AppOpenResult{Code: state, ID: id}, nil
			})
			value, exit, err := openPeerApp(context.Background(), client, []string{"open-status", "B", id})
			want := ExitUnsafeState
			if state == "window_ready" {
				want = ExitOK
			}
			if err != nil || exit != want || value.(appOpenResult).Code != state || client.calls != 1 {
				t.Fatal("status query treated launch acknowledgement as window readiness", value, exit, err)
			}
		})
	}
}

func TestCLIAppOpenDoesNotFollowUnverifiableStartReply(t *testing.T) {
	for _, reply := range []localapi.AppOpenResult{
		{Code: "queued", ID: "invalid"},
		{Code: "window_ready", ID: strings.Repeat("b", 32)},
		{Code: "launch_requested"},
	} {
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			return reply, nil
		})
		value, exit, err := openPeerApp(context.Background(), client, []string{"open", "B"})
		result := value.(appOpenResult)
		if err != nil || exit != ExitUnsafeState || result.Code != "outcome_unknown" || result.ID != "" || client.calls != 1 {
			t.Fatal("unverifiable request was followed or reported success", reply, result, exit, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := scriptedAppOpenClient(nil)
	value, exit, err := openPeerApp(ctx, client, []string{"open", "B"})
	if err != nil || exit != ExitUnsafeState || value.(appOpenResult).Code != "canceled" || client.calls != 0 {
		t.Fatal("already canceled caller launched an app", value, exit, err)
	}
}
