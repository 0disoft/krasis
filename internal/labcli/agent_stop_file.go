package labcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/securefs"
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
	directory, err := securefs.OpenDirectory(filepath.Dir(path))
	if err != nil {
		return nil, nil, fmt.Errorf("open agent stop directory: %w", err)
	}
	name := filepath.Base(path)
	if file, err := directory.Open(name); err == nil {
		_ = file.Close()
		_ = directory.Close()
		return nil, nil, errors.New("agent stop file already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = directory.Close()
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
				file, err := directory.Open(name)
				switch {
				case err == nil:
					_ = file.Close()
					_ = directory.Remove(name)
					cancel(errAgentStopRequested)
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
		_ = directory.Remove(name)
		_ = directory.Close()
		cancel(context.Canceled)
	}
	return ctx, closeMonitor, nil
}
