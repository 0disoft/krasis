package localapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Decode from a bounded stream without retaining another full base64 JSON copy.
// The sentinel counts trailing whitespace too; the second decode must reach EOF.
func decodeResponse(source io.Reader, target any, limit int64) error {
	limited := &io.LimitedReader{R: source, N: limit + 1}
	body := &responseReadObserver{source: limited}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	phase := "decode response"
	err := decoder.Decode(target)
	if err == nil {
		phase = "trailing response"
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			err = errors.New("unexpected trailing JSON")
		}
	}
	if err != nil {
		// Finish the bounded read just as ReadAll did, preserving late read
		// failures and size-limit precedence even after an early JSON error.
		_, _ = io.Copy(io.Discard, body)
	}
	// A reader can return both a complete JSON object and a transport failure.
	// Preserve the read failure even when Decoder returns a successful object.
	if body.err != nil {
		return &requestError{ErrTransport, "read response", body.err}
	}
	if limited.N == 0 {
		return fmt.Errorf("%w: response exceeds size limit", ErrProtocol)
	}
	if err != nil {
		return &requestError{ErrProtocol, phase, err}
	}
	return nil
}

type responseReadObserver struct {
	source io.Reader
	err    error
}

func (r *responseReadObserver) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if err != nil && err != io.EOF && r.err == nil {
		r.err = err
	}
	return n, err
}
