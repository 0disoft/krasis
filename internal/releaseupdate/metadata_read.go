package releaseupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	metadataReadTimeout    = 10 * time.Second
	metadataAttemptTimeout = 3 * time.Second
	metadataReadAttempts   = 3
	metadataRetryBase      = 250 * time.Millisecond
)

// Only this fixed, small GET is retried. Downloads, publication, installation
// and peer commands never enter this helper, regardless of their HTTP method.
func readSignedMetadata(parent context.Context, client *http.Client) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, metadataReadTimeout)
	defer cancel()
	var last error
	for attempt := 0; attempt < metadataReadAttempts && ctx.Err() == nil; attempt++ {
		request, stop := context.WithTimeout(ctx, metadataAttemptTimeout)
		data, err := fetch(request, client, FeedURL, 16384)
		// Reject a late read even if its transport returned bytes after cancellation.
		if err == nil {
			err = request.Err()
		}
		stop()
		if err == nil && ctx.Err() == nil {
			return data, nil
		}
		last = err
		if ctx.Err() != nil || !retryMetadataRead(err) || attempt == metadataReadAttempts-1 {
			break
		}
		delay := metadataRetryBase << attempt
		var status *HTTPError
		if errors.As(err, &status) {
			delay = max(delay, status.RetryAfter)
		}
		deadline, _ := ctx.Deadline()
		if delay >= time.Until(deadline) {
			break // Never shorten Retry-After to squeeze in another request.
		}
		if err := waitMetadataRetry(ctx, delay); err != nil {
			last = errors.Join(last, err)
			break
		}
	}
	return nil, fmt.Errorf("read signed release metadata: %w", errors.Join(last, ctx.Err(), context.Cause(ctx)))
}

func retryMetadataRead(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrInvalid) {
		return false
	}
	var status *HTTPError
	if errors.As(err, &status) {
		switch status.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	var network net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.As(err, &network) && (network.Timeout() || network.Temporary())
}

func waitMetadataRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
