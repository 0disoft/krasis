package releaseupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUnconfiguredKeyNeverContactsNetwork(t *testing.T) {
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("network with no trusted key"); return nil, nil })}
	if _, err := Check(context.Background(), c, ""); !errors.Is(err, ErrUnconfigured) {
		t.Fatal(err)
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
