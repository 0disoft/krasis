package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/0disoft/krasis/internal/peerupdate"
)

func BenchmarkClipboardResponse8MiB(b *testing.B) {
	data, err := json.Marshal(ClipboardResult{Code: "ok", Value: peerupdate.ClipboardValue{
		Format: "files", Data: make([]byte, 8<<20),
	}})
	if err != nil {
		b.Fatal(err)
	}
	client := &Client{
		credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1"}, Token: "test-only"},
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": {"application/json"}},
				Body:   io.NopCloser(bytes.NewReader(data))}, nil
		})},
	}
	b.ReportAllocs()
	for b.Loop() {
		var value ClipboardResult
		if err := client.requestHeadersLimit(context.Background(), http.MethodPost, "/v1/clipboard", nil, &value, nil, maxClipboardJSONBytes); err != nil {
			b.Fatal(err)
		}
		if len(value.Value.Data) != 8<<20 {
			b.Fatal("decoded payload changed")
		}
	}
}
