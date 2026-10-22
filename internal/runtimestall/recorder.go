// Package runtimestall captures bounded local latency and failure traces.
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
	request   chan bool // true: terminal failure; false: optional latency sample.
	done      chan struct{}
	cancel    context.CancelFunc
	requested atomic.Bool
	sampled   atomic.Bool
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
	r := &Recorder{request: make(chan bool, 2), done: make(chan struct{}), cancel: cancel}
	event("runtime_trace_enabled")
	go func() {
		defer cancel()
		defer close(r.done)
		defer source.Stop()
		timer := time.NewTimer(ttl)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				event("runtime_trace_expired")
				return
			case terminal := <-r.request:
				if !terminal && r.requested.Load() {
					continue // A queued sample cannot precede a requested failure.
				}
				if err := save(source); err != nil {
					event(saveFailureCode(err))
				} else if terminal {
					event("runtime_trace_saved")
				} else {
					event("runtime_trace_sample_saved")
				}
				if terminal {
					return
				}
			}
		}
	}()
	return r
}

// Request captures the final failure and cannot wait for flushing or disk I/O.
func (r *Recorder) Request() {
	if r == nil || !r.requested.CompareAndSwap(false, true) {
		return
	}
	select {
	case r.request <- true:
	default:
	}
}

// RequestSample retains one latency sample without consuming the final failure
// snapshot. Both requests are nonblocking and use the same bounded recorder and
// existing output slots; the final failure replaces the earlier sample.
func (r *Recorder) RequestSample() {
	if r == nil || r.requested.Load() || !r.sampled.CompareAndSwap(false, true) {
		return
	}
	select {
	case r.request <- false:
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
