package localapi

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDecodeResponseBoundariesAndStrictJSON(t *testing.T) {
	const object = `{"value":1}`
	for _, tc := range []struct {
		name, data string
		limit      int64
		wantError  bool
	}{
		{"exact-limit", object, int64(len(object)), false},
		{"whitespace-at-limit", object + " \n", int64(len(object) + 2), false},
		{"whitespace-past-limit", object + " ", int64(len(object)), true},
		{"truncated-at-limit", object, int64(len(object) - 1), true},
		{"trailing-object", object + object, 128, true},
		{"trailing-null", object + "null", 128, true},
		{"trailing-malformed", object + "{", 128, true},
		{"unknown-field", `{"other":1}`, 128, true},
		{"empty", "", 128, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value struct {
				Value int `json:"value"`
			}
			err := decodeResponse(strings.NewReader(tc.data), &value, tc.limit)
			if tc.wantError {
				if !errors.Is(err, ErrProtocol) {
					t.Fatalf("decodeResponse = %v; want ErrProtocol", err)
				}
			} else if err != nil || value.Value != 1 {
				t.Fatalf("decodeResponse = %+v, %v", value, err)
			}
		})
	}
}

type responseDataAndError struct{ data string }

func (r *responseDataAndError) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, context.Canceled
}

func TestDecodeResponsePreservesTransportFailureWithCompleteJSON(t *testing.T) {
	for name, source := range map[string]io.Reader{
		"before-json":          &failedResponseBody{},
		"with-json":            &responseDataAndError{data: `{"value":1}`},
		"after-json":           io.MultiReader(strings.NewReader(`{"value":1}`), &failedResponseBody{}),
		"with-overflow":        &responseDataAndError{data: `{"value":1}`},
		"after-malformed-json": io.MultiReader(strings.NewReader(`{"value":]}`), &failedResponseBody{}),
		"after-trailing-json":  io.MultiReader(strings.NewReader(`{"value":1} {}`), &failedResponseBody{}),
	} {
		t.Run(name, func(t *testing.T) {
			limit := int64(128)
			if name == "with-overflow" {
				limit = 2
			}
			var value map[string]int
			err := decodeResponse(source, &value, limit)
			if !errors.Is(err, ErrTransport) || !errors.Is(err, context.Canceled) {
				t.Fatalf("transport failure misclassified: %v", err)
			}
			if err.Error() != "local API transport failed: read response" {
				t.Fatalf("transport details exposed: %v", err)
			}
		})
	}
}

func TestDecodeResponseReadsAtMostLimitPlusSentinel(t *testing.T) {
	const limit = 1024
	source := strings.NewReader(`{"value":1}` + strings.Repeat(" ", 2*limit))
	before := source.Len()
	var value map[string]int
	if err := decodeResponse(source, &value, limit); !errors.Is(err, ErrProtocol) {
		t.Fatalf("oversized whitespace accepted: %v", err)
	}
	if read := before - source.Len(); read != limit+1 {
		t.Fatalf("read %d bytes; want %d", read, limit+1)
	}
}
