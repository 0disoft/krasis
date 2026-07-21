package labcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

var errAgentStopRequested = errors.New("agent stop requested")

const agentStopFilePollInterval = 100 * time.Millisecond

func monitorAgentStopFile(parent context.Context, path string) (context.Context, func(), error) {
	if parent == nil {
		return nil, nil, errors.New("agent context is required")
	}
	if path == "" {
		return parent, func() {}, nil
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, nil, errors.New("agent stop file already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancelCause(parent)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(agentStopFilePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				cancel(context.Cause(parent))
				return
			case <-stop:
				return
			case <-ticker.C:
				info, err := os.Lstat(path)
				switch {
				case err == nil:
					if info.Mode().IsRegular() {
						_ = os.Remove(path)
						cancel(errAgentStopRequested)
						return
					}
					cancel(errors.New("agent stop path is not a regular file"))
					return
				case errors.Is(err, os.ErrNotExist):
					continue
				default:
					cancel(fmt.Errorf("inspect agent stop file: %w", err))
					return
				}
			}
		}
	}()

	closeMonitor := func() {
		close(stop)
		<-done
		_ = os.Remove(path)
		cancel(context.Canceled)
	}
	return ctx, closeMonitor, nil
}
