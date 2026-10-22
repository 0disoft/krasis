package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRequestFailurePreservesCauseWithoutReplayingWritesOrLeakingDetails(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("private endpoint or credential detail")} {
		calls := 0
		client := &Client{
			credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1"}, Token: "test-only"},
			http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, cause
			})},
		}
		var result any
		err := client.request(context.Background(), http.MethodPost, "/v1/app-open", nil, &result)
		if !errors.Is(err, ErrUnavailable) || !errors.Is(err, cause) || calls != 1 {
			t.Fatal("cause was lost or write was replayed", err, calls)
		}
		if err.Error() != "resident agent is unavailable: request failed" {
			t.Fatal("display error exposed native details", err)
		}
	}
}

type failedResponseBody struct{ closed bool }

func (*failedResponseBody) Read([]byte) (int, error) { return 0, context.Canceled }
func (b *failedResponseBody) Close() error           { b.closed = true; return nil }

func TestResponseErrorsKeepCausesAndCloseBodies(t *testing.T) {
	for _, phase := range []string{"read", "decode", "trailing"} {
		body := &failedResponseBody{}
		var source io.ReadCloser = body
		if phase == "decode" {
			source = io.NopCloser(strings.NewReader(`{"value":]}`))
		} else if phase == "trailing" {
			source = io.NopCloser(strings.NewReader(`{"value":1} {`))
		}
		client := &Client{
			credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1"}, Token: "test-only"},
			http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: source}, nil
			})},
		}
		var result map[string]any
		err := client.request(context.Background(), http.MethodGet, "/v1/status", nil, &result)
		switch phase {
		case "read":
			if !errors.Is(err, ErrTransport) || !errors.Is(err, context.Canceled) || !body.closed {
				t.Fatal("read failure lost cause or body ownership", err, body.closed)
			}
		case "decode":
			var syntax *json.SyntaxError
			if !errors.Is(err, ErrProtocol) || !errors.As(err, &syntax) {
				t.Fatal("decoder cause lost", err)
			}
		case "trailing":
			if !errors.Is(err, ErrProtocol) || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("trailing decoder cause lost", err)
			}
		}
	}
}
