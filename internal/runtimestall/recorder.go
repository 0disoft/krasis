// Package runtimestall captures one local execution trace after a runtime stall.
// It never annotates traces with input, clipboard, peer or credential values.
package runtimestall

import (
	"context"
	"io"
	"os"
	"runtime/trace"
	"sync/atomic"
	"time"
)

const Environment = "KRASIS_RUNTIME_TRACE"
const lifetime = 15 * time.Minute

type traceSource interface {
	Start() error
	Stop()
	WriteTo(io.Writer) (int64, error)
}

type Recorder struct {
	request   chan struct{}
	done      chan struct{}
	cancel    context.CancelFunc
	requested atomic.Bool
}

// Start is opt-in for this process only; ordinary and service launches stay off.
func Start(ctx context.Context, base string, event func(string)) *Recorder {
	if os.Getenv(Environment) != "1" {
		return nil
	}
	source := trace.NewFlightRecorder(trace.FlightRecorderConfig{MinAge: 10 * time.Second, MaxBytes: 2 << 20})
	return start(ctx, source, func(s traceSource) error { return save(base, s) }, event, lifetime)
}

func start(parent context.Context, source traceSource, save func(traceSource) error, event func(string), ttl time.Duration) *Recorder {
	if err := source.Start(); err != nil {
		event("runtime_trace_start_failed")
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	r := &Recorder{request: make(chan struct{}, 1), done: make(chan struct{}), cancel: cancel}
	event("runtime_trace_enabled")
	go func() {
		defer cancel()
		defer close(r.done)
		defer source.Stop()
		timer := time.NewTimer(ttl)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			event("runtime_trace_expired")
			return
		case <-r.request:
			if err := save(source); err != nil {
				event(saveFailureCode(err))
			} else {
				event("runtime_trace_saved")
			}
		}
	}()
	return r
}

// Request cannot wait for trace flushing, runtime sampling or disk I/O.
func (r *Recorder) Request() {
	if r == nil || !r.requested.CompareAndSwap(false, true) {
		return
	}
	select {
	case r.request <- struct{}{}:
	default:
	}
}

func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
	}
}
