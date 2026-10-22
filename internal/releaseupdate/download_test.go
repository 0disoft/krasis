package releaseupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUnconfiguredKeyNeverContactsNetwork(t *testing.T) {
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("network with no trusted key"); return nil, nil })}
	if _, err := Check(context.Background(), c, ""); !errors.Is(err, ErrUnconfigured) {
		t.Fatal(err)
	}
}

func TestFetchPreservesHTTPStatusAndBoundedRetryAfter(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500, 502, 503, 504} {
		calls := 0
		c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"30"}}, Body: io.NopCloser(strings.NewReader("private response"))}, nil
		})}
		_, err := fetch(context.Background(), c, "https://github.com/private-asset", 64)
		var failure *HTTPError
		if !errors.As(err, &failure) || failure.StatusCode != status || failure.RetryAfter != 30*time.Second || calls != 1 {
			t.Fatal("HTTP metadata lost or retry silently added", err, calls)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("download failure exposed body or URL", err)
		}
	}
}

func TestRetryAfterBoundsAndDateParsing(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		value string
		want  time.Duration
	}{
		{"", 0}, {"garbage", 0}, {"-1", 0}, {"0", 0}, {"1", time.Second},
		{"299", 299 * time.Second}, {"300", maxRetryAfter}, {"301", maxRetryAfter},
		{"9223372036854775807", maxRetryAfter},
		{now.Add(-time.Second).Format(http.TimeFormat), 0},
		{now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{now.Add(time.Hour).Format(http.TimeFormat), maxRetryAfter},
	} {
		if got := retryAfter(item.value, now); got != item.want {
			t.Fatal(item.value, got, item.want)
		}
	}
}

func TestFetchDistinguishesDeclaredOversizeFromHTTPFailure(t *testing.T) {
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 65, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	if _, err := fetch(context.Background(), c, "https://github.com/asset", 64); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized body was treated as a transient HTTP failure", err)
	}
}
func TestDownloadRejectsOverflowAndWrongHashBeforeBundleParsing(t *testing.T) {
	for _, body := range []string{"bad", "much too long"} {
		c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		m := manifest()
		m.Size = 3
		if _, err := Download(context.Background(), c, m); err == nil {
			t.Fatal("invalid bytes accepted")
		}
	}
}
func TestRedirectsStayOnHTTPSReleaseStorage(t *testing.T) {
	c := Client()
	for _, raw := range []string{"http://github.com/0disoft/krasis", "https://evil.example/update", "https://github.com:443/update", "https://user@github.com/update"} {
		u, _ := url.Parse(raw)
		if c.CheckRedirect(&http.Request{URL: u}, nil) == nil {
			t.Fatal(raw)
		}
	}
	u, _ := url.Parse("https://release-assets.githubusercontent.com/file")
	if err := c.CheckRedirect(&http.Request{URL: u}, nil); err != nil {
		t.Fatal(err)
	}
}
