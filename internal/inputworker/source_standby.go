package inputworker

import (
	"context"
	"io"
	"time"
)

// Standby owns no suppression or remote pressed state. Broker/GC scheduling
// must not give it the same idle policy as active capture. Process presence is
// independently watched, while active input and partial frames keep 2s bounds.
const sourceStandbyTimeout = 5 * IdleTimeout

func sourceReadTimeout(started, active bool) time.Duration {
	if !started || active {
		return IdleTimeout
	}
	return sourceStandbyTimeout
}

type sourceFrameReader struct {
	io.Reader
	setDeadline func(time.Time) error
	deadline    time.Time
	started     bool
}

func (r *sourceFrameReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	if n > 0 && !r.started {
		r.started = true
		if n < len(data) {
			limit := time.Now().Add(IdleTimeout)
			if r.deadline.Before(limit) {
				limit = r.deadline
			}
			if failure := r.setDeadline(limit); failure != nil {
				return n, failure
			}
		}
	}
	return n, err
}

func watchSourcePresence(ctx context.Context, check func() error) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			if check() != nil {
				return ErrAuthority
			}
		}
	}
}
