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

type peerConnectTrustClient struct {
	*appOpenClient
	trustRead func(context.Context) (localapi.Trust, error)
}

func (c peerConnectTrustClient) Trust(ctx context.Context) (localapi.Trust, error) {
	return c.trustRead(ctx)
}

func TestCLIConnectPreCancellationPerformsNoIO(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reads := 0
	client := scriptedAppOpenClient(func(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
		t.Fatal("canceled command submitted work")
		return localapi.AppOpenResult{}, nil
	})
	observed := peerConnectTrustClient{client, func(context.Context) (localapi.Trust, error) {
		reads++
		return client.stubClient.Trust(context.Background())
	}}
	_, exit, err := connectPeerApp(ctx, observed, []string{"connect", "B"})
	if reads != 0 || client.calls != 0 || exit != ExitUnsafeState || err != nil {
		t.Fatalf("canceled command performed I/O: reads=%d calls=%d exit=%d err=%v", reads, client.calls, exit, err)
	}
}

func TestCLIConnectDeadlineBeforeAndAfterEachOperation(t *testing.T) {
	id := strings.Repeat("b", 32)
	for _, stage := range []string{"trust", "submit", "status"} {
		for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			t.Run(stage+"/"+offset.String(), func(t *testing.T) {
				now := time.Now()
				reads, starts, polls, timers, stops := 0, 0, 0, 0, 0
				advance := func(ctx context.Context) {
					if ctx.Err() != nil {
						t.Fatal("real timeout used by boundary test")
					}
					deadline, _ := ctx.Deadline()
					now = deadline.Add(offset)
				}
				client := scriptedAppOpenClient(func(ctx context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
					switch r.Action {
					case "CONNECT":
						starts++
						if stage == "submit" {
							advance(ctx)
						}
						return localapi.AppOpenResult{Code: "queued", ID: id}, nil
					case "CONNECT_STATUS":
						polls++
						if r.ID != id {
							t.Fatal("wrong receipt", r.ID)
						}
						if stage == "status" {
							advance(ctx)
						}
						return localapi.AppOpenResult{Code: "connected", ID: id}, nil
					default:
						t.Fatal("unexpected I/O", r.Action)
						return localapi.AppOpenResult{}, nil
					}
				})
				observed := peerConnectTrustClient{client, func(ctx context.Context) (localapi.Trust, error) {
					reads++
					if stage == "trust" {
						advance(ctx)
					}
					return client.stubClient.Trust(ctx)
				}}
				value, exit, err := connectPeerApp(context.Background(), observed, []string{"connect", "B"}, peerConnectOptions{
					now: func() time.Time { return now },
					ticker: func(interval time.Duration) (<-chan time.Time, func()) {
						if interval != time.Second {
							t.Fatal("poll interval changed", interval)
						}
						timers++
						return nil, func() { stops++ }
					},
				})
				result := value.(appOpenResult)
				wantCode, wantExit, wantStarts, wantPolls, wantTimers := "connected", ExitOK, 1, 1, 1
				if offset >= 0 {
					wantExit = ExitUnsafeState
					switch stage {
					case "trust":
						wantCode, wantStarts, wantPolls, wantTimers = "connection_unconfirmed", 0, 0, 0
					case "submit":
						wantCode, wantPolls, wantTimers = "connection_unconfirmed", 0, 0
					case "status":
						wantCode = "outcome_unknown"
					}
				}
				if err != nil || exit != wantExit || result.Code != wantCode || reads != 1 || starts != wantStarts || polls != wantPolls || timers != wantTimers || stops != timers || starts > 0 && result.ID != id {
					t.Fatalf("result=%+v exit=%d err=%v reads=%d starts=%d polls=%d timers=%d stops=%d", result, exit, err, reads, starts, polls, timers, stops)
				}
			})
		}
	}
}

func TestCLIConnectOptionalAbortPropagatesWithoutReplaying(t *testing.T) {
	for _, stage := range []string{"before", "trust", "submit", "status"} {
		t.Run(stage, func(t *testing.T) {
			signal, abort := context.WithCancel(context.Background())
			defer abort()
			if stage == "before" {
				abort()
			}
			reads, starts, polls := 0, 0, 0
			id := strings.Repeat("b", 32)
			var nativeContext context.Context
			client := scriptedAppOpenClient(func(ctx context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
				nativeContext = ctx
				if r.Action == "CONNECT" {
					starts++
					if stage == "submit" {
						abort()
					}
					return localapi.AppOpenResult{Code: "queued", ID: id}, nil
				}
				polls++
				abort()
				return localapi.AppOpenResult{Code: "connected", ID: id}, nil
			})
			observed := peerConnectTrustClient{client, func(ctx context.Context) (localapi.Trust, error) {
				nativeContext = ctx
				reads++
				if stage == "trust" {
					abort()
				}
				return client.stubClient.Trust(ctx)
			}}
			value, exit, err := connectPeerApp(context.Background(), observed, []string{"connect", "B"}, peerConnectOptions{signal: signal})
			result := value.(appOpenResult)
			wantReads, wantStarts, wantPolls := 1, 0, 0
			if stage == "before" {
				wantReads = 0
			}
			if stage == "submit" || stage == "status" {
				wantStarts = 1
			}
			if stage == "status" {
				wantPolls = 1
			}
			if err != nil || exit != ExitUnsafeState || result.Code == "connected" || reads != wantReads || starts != wantStarts || polls != wantPolls || starts > 0 && result.ID != id || nativeContext != nil && nativeContext.Err() == nil {
				t.Fatalf("result=%+v exit=%d err=%v reads=%d starts=%d polls=%d", result, exit, err, reads, starts, polls)
			}
		})
	}
}

func TestCLIConnectAbortInterruptsOutstandingNativeRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		signal, abort := context.WithCancel(context.Background())
		defer abort()
		id := strings.Repeat("b", 32)
		started := make(chan struct{})
		starts, polls := 0, 0
		client := scriptedAppOpenClient(func(ctx context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			if r.Action == "CONNECT" {
				starts++
				return localapi.AppOpenResult{Code: "queued", ID: id}, nil
			}
			polls++
			close(started)
			<-ctx.Done()
			return localapi.AppOpenResult{}, ctx.Err()
		})
		done := make(chan struct{})
		go func() {
			defer close(done)
			value, exit, err := connectPeerApp(context.Background(), client, []string{"connect", "B"}, peerConnectOptions{signal: signal})
			result := value.(appOpenResult)
			if err != nil || exit != ExitUnsafeState || result.Code != "outcome_unknown" || result.ID != id {
				t.Errorf("result=%+v exit=%d err=%v", result, exit, err)
			}
		}()
		<-started
		abort()
		<-done
		if starts != 1 || polls != 1 {
			t.Fatal("abort replayed native work", starts, polls)
		}
	})
}

func TestCLIConnectIOFailuresNeverRetryAndStopPollingTimer(t *testing.T) {
	for _, stage := range []string{"trust", "submit", "status"} {
		t.Run(stage, func(t *testing.T) {
			reads, starts, polls, timers, stops := 0, 0, 0, 0, 0
			fault := errors.New("transport failed")
			id := strings.Repeat("b", 32)
			client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
				if r.Action == "CONNECT" {
					starts++
					if stage == "submit" {
						return localapi.AppOpenResult{}, fault
					}
					return localapi.AppOpenResult{Code: "queued", ID: id}, nil
				}
				polls++
				return localapi.AppOpenResult{}, fault
			})
			observed := peerConnectTrustClient{client, func(ctx context.Context) (localapi.Trust, error) {
				reads++
				if stage == "trust" {
					return localapi.Trust{}, fault
				}
				return client.stubClient.Trust(ctx)
			}}
			value, exit, err := connectPeerApp(context.Background(), observed, []string{"connect", "B"}, peerConnectOptions{
				ticker: func(time.Duration) (<-chan time.Time, func()) { timers++; return nil, func() { stops++ } },
			})
			if stage == "trust" {
				if value != nil || err != fault || exit != ExitInternal || reads != 1 || starts != 0 || polls != 0 || timers != 0 {
					t.Fatal("trust failure started native work", value, err, exit, reads, starts, polls, timers)
				}
				return
			}
			result := value.(appOpenResult)
			wantPolls := 0
			if stage == "status" {
				wantPolls = 1
			}
			if err != nil || exit != ExitUnsafeState || result.Code != "outcome_unknown" || starts != 1 || polls != wantPolls || timers != wantPolls || stops != timers {
				t.Fatal("uncertain native call replayed or leaked timer", result, err, exit, starts, polls, timers, stops)
			}
		})
	}
}

func TestCLIConnectStatusPreCancellationPreservesReceipt(t *testing.T) {
	id := strings.Repeat("b", 32)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := scriptedAppOpenClient(func(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
		t.Fatal("canceled status performed native I/O")
		return localapi.AppOpenResult{}, nil
	})
	value, exit, err := connectPeerApp(ctx, client, []string{"connect-status", "B", id})
	result := value.(appOpenResult)
	if err != nil || exit != ExitUnsafeState || result.ID != id || result.Code != "connection_unconfirmed" || client.calls != 0 {
		t.Fatalf("result=%+v exit=%d err=%v", result, exit, err)
	}
}

func TestCLIConnectWaitsForExactObservedConnectionAndNeverRetriesStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		id := strings.Repeat("b", 32)
		starts, polls := 0, 0
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			if r.Action == "CONNECT" {
				starts++
				return localapi.AppOpenResult{Code: "queued", ID: id}, nil
			}
			if r.Action != "CONNECT_STATUS" || r.ID != id {
				t.Fatal("wrong request")
			}
			polls++
			code := "accepted"
			if polls == 3 {
				code = "connected"
			}
			return localapi.AppOpenResult{Code: code, ID: id}, nil
		})
		var out, errs bytes.Buffer
		if exit := run(context.Background(), []string{"app", "connect", "B", "--json"}, &out, &errs, testDependencies(client)); exit != ExitOK || !strings.Contains(out.String(), `"code":"connected"`) || starts != 1 || polls != 3 {
			t.Fatal(exit, out.String(), errs.String(), starts, polls)
		}
	})
}

func TestCLIConnectUnknownDeliveryAndStatusNeverSubmitAgain(t *testing.T) {
	for _, query := range []bool{false, true} {
		starts := 0
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			if r.Action == "CONNECT" {
				starts++
				return localapi.AppOpenResult{}, errors.New("lost response")
			}
			return localapi.AppOpenResult{Code: "connected", ID: strings.Repeat("c", 32)}, nil
		})
		args := []string{"app", "connect", "B", "--json"}
		if query {
			args = []string{"app", "connect-status", "B", strings.Repeat("b", 32), "--json"}
		}
		var out, errs bytes.Buffer
		if exit := run(context.Background(), args, &out, &errs, testDependencies(client)); exit == ExitOK || !strings.Contains(out.String(), "outcome_unknown") {
			t.Fatal(exit, out.String())
		}
		if (!query && starts != 1) || (query && starts != 0) {
			t.Fatal("resubmitted", starts)
		}
	}
}
