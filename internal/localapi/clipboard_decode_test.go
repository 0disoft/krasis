package localapi

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestClipboardStreamKeepsStrictJSONValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"whitespace", " \n{}\t\r\n ", true},
		{"second_object", "{} {}", false},
		{"trailing_number", "{} 1", false},
		{"trailing_garbage", "{}x", false},
		{"non_json_whitespace", "{}\v", false},
		{"unknown_field", `{"unexpected":true}`, false},
		{"truncated", `{"action":`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value ClipboardRequest
			if err := decodeClipboardRequest(strings.NewReader(tc.body), &value); (err == nil) != tc.valid {
				t.Fatalf("accepted=%t, want %t: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestClipboardStreamCountsTrailingWhitespaceAgainstLimit(t *testing.T) {
	for _, extra := range []int64{0, 1, 1024} {
		t.Run(fmt.Sprintf("bytes_over_limit_%d", extra), func(t *testing.T) {
			length := int64(maxClipboardJSONBytes) + extra
			source := &io.LimitedReader{R: io.MultiReader(strings.NewReader("{}"), clipboardPaddingReader{}), N: length}
			var value ClipboardRequest
			err := decodeClipboardRequest(source, &value)
			if (err == nil) != (extra == 0) {
				t.Fatalf("length=%d: %v", length, err)
			}
			if consumed := length - source.N; consumed > int64(maxClipboardJSONBytes)+1 {
				t.Fatalf("read %d bytes past the bounded admission", consumed)
			}
		})
	}
}

func TestClipboardStreamPreservesReadFailureAfterObject(t *testing.T) {
	var value ClipboardRequest
	err := decodeClipboardRequest(iotest.TimeoutReader(strings.NewReader("{}")), &value)
	if !errors.Is(err, iotest.ErrTimeout) {
		t.Fatalf("lost trailing body read failure: %v", err)
	}
}

type clipboardPaddingReader struct{}

func (clipboardPaddingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}
