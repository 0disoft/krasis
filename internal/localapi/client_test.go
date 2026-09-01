package localapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientAuthenticatesAndValidatesSchemas(t *testing.T) {
	control := filepath.Join(protectedRoot(t), "control")
	server, err := Listen(control, &stubOperations{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	client, err := OpenClient(control)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.Status(context.Background())
	if err != nil || status.State != "LOCAL" {
		t.Fatalf("Status() = %+v, %v", status, err)
	}
	trust, err := client.Trust(context.Background())
	if err != nil || trust.Peers == nil {
		t.Fatalf("Trust() = %+v, %v", trust, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsIncompatibleResponseSchema(t *testing.T) {
	client := &Client{
		credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1234"}, Token: strings.Repeat("a", 64)},
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"schemaVersion":"krasis.agent.status/v2","state":"LOCAL","actions":[],"deviceFingerprint":"","identityGeneration":0}`)),
			}, nil
		})},
	}
	if _, err := client.Status(context.Background()); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Status() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestClientClassifiesUnavailableAndAuthenticationFailures(t *testing.T) {
	if _, err := OpenClient(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("OpenClient() error = %v", err)
	}
	client := &Client{
		credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1"}, Token: "invalid"},
		http:        http.DefaultClient,
	}
	if _, err := client.Status(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Status() error = %v", err)
	}
}
