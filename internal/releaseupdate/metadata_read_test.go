package releaseupdate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"
)

func TestSignedMetadataRecoversTransientReadWithBackoff(t *testing.T) {
	envelope, key := signed(t, manifest())
	synctest.Test(t, func(t *testing.T) {
		var starts []time.Time
		var bodies []*metadataTestBody
		client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet || r.URL.String() != FeedURL {
				t.Fatal("metadata retry escaped the fixed read endpoint")
			}
			for _, body := range bodies {
				if !body.closed {
					t.Fatal("retry retained the previous response body")
				}
			}
			starts = append(starts, time.Now())
			body := &metadataTestBody{Reader: bytes.NewReader(envelope)}
			bodies = append(bodies, body)
			status := http.StatusServiceUnavailable
			if len(starts) == 3 {
				status = http.StatusOK
			}
			return &http.Response{StatusCode: status, Body: body}, nil
		})}
		got, raw, err := CheckSigned(context.Background(), client, key)
		if err != nil || got != manifest() || !bytes.Equal(raw, envelope) {
			t.Fatalf("transient metadata read did not recover: %v", err)
		}
		if len(starts) != 3 || starts[1].Sub(starts[0]) != 250*time.Millisecond || starts[2].Sub(starts[1]) != 500*time.Millisecond {
			t.Fatalf("unexpected request count or exponential backoff: %v", starts)
		}
		if !bodies[2].closed {
			t.Fatal("successful response body retained")
		}
	})
}

type metadataTestBody struct {
	io.Reader
	closed bool
}

func (b *metadataTestBody) Close() error { b.closed = true; return nil }

func TestSignedMetadataStallIsBoundedAndKeepsCause(t *testing.T) {
	_, key := signed(t, manifest())
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		cause := errors.New("transport stalled")
		client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			<-r.Context().Done()
			return nil, errors.Join(cause, r.Context().Err())
		})}
		started := time.Now()
		_, _, err := CheckSigned(context.Background(), client, key)
		if calls != 3 || time.Since(started) > 10*time.Second || !errors.Is(err, cause) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unbounded read or lost cause: calls=%d elapsed=%v error=%v", calls, time.Since(started), err)
		}
	})
}

func TestSignedMetadataCancellationDuringBackoffPreservesHTTPFailure(t *testing.T) {
	_, key := signed(t, manifest())
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		body := &metadataTestBody{Reader: bytes.NewReader(nil)}
		client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil
		})}
		done := make(chan error, 1)
		go func() { _, _, err := CheckSigned(ctx, client, key); done <- err }()
		synctest.Wait()
		cancel()
		err := <-done
		var status *HTTPError
		if calls != 1 || !body.closed || !errors.Is(err, context.Canceled) || !errors.As(err, &status) || status.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("cancellation retried or lost the original HTTP cause: calls=%d error=%v", calls, err)
		}
	})
}

func TestSignedMetadataRetryAfterRespectsTotalBudget(t *testing.T) {
	envelope, key := signed(t, manifest())
	for _, item := range []struct {
		name, retryAfter string
		calls            int
		elapsed          time.Duration
	}{
		{"within_budget", "1", 2, time.Second},
		{"at_budget", "10", 1, 0},
		{"beyond_budget", "30", 1, 0},
	} {
		t.Run(item.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
					calls++
					status := http.StatusOK
					if calls == 1 {
						status = http.StatusTooManyRequests
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {item.retryAfter}}, Body: io.NopCloser(bytes.NewReader(envelope))}, nil
				})}
				started := time.Now()
				_, _, err := CheckSigned(context.Background(), client, key)
				if calls != item.calls || time.Since(started) != item.elapsed {
					t.Fatalf("Retry-After violated: calls=%d elapsed=%v", calls, time.Since(started))
				}
				if item.calls == 2 && err != nil {
					t.Fatal(err)
				}
				if item.calls == 1 {
					var status *HTTPError
					if !errors.As(err, &status) || status.StatusCode != http.StatusTooManyRequests {
						t.Fatalf("rate limit cause lost: %v", err)
					}
				}
			})
		})
	}
}

func TestSignedMetadataNeverRetriesPermanentOrInvalidResponse(t *testing.T) {
	_, key := signed(t, manifest())
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusOK} {
		calls := 0
		client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader([]byte("invalid signed envelope")))}, nil
		})}
		if _, _, err := CheckSigned(context.Background(), client, key); err == nil || calls != 1 {
			t.Fatalf("permanent HTTP/signature failure retried: status=%d calls=%d error=%v", status, calls, err)
		}
	}
}

func TestSignedMetadataPreCanceledContextMakesNoRequest(t *testing.T) {
	_, key := signed(t, manifest())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("canceled metadata read contacted network")
		return nil, nil
	})}
	if _, _, err := CheckSigned(ctx, client, key); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
