package inputworker

import (
	"context"
	"time"
)

// Timing separates input from idle maintenance with coarse elapsed-time buckets,
// never keyboard, mouse or wheel operation kinds,
// input values, identities or error strings. It is not an authority signal.
var timingStages = [...]string{"client_lock", "client_authority", "client_write", "client_response", "worker_authority", "worker_scope", "worker_apply"}

func timingBucket(elapsed time.Duration) byte {
	switch {
	case elapsed < 0:
		return 0
	case elapsed < time.Millisecond:
		return 1
	case elapsed < 10*time.Millisecond:
		return 2
	case elapsed < 100*time.Millisecond:
		return 3
	case elapsed < time.Second:
		return 4
	default:
		return 5
	}
}

func timingCode(stage int, bucket byte) string {
	if stage < 0 || stage >= len(timingStages) || bucket < 1 || bucket > 5 {
		return ""
	}
	suffix := [...]string{"", "lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"}
	return "installed_input_timing_" + timingStages[stage] + "_" + suffix[bucket]
}

type timingSampler struct {
	emit      func(string)
	heartbeat bool
	started   time.Time
	maximum   [7]byte
}

// Called under the client mutex. Emit must be nonblocking and must not reenter
// the client. At most seven aggregate codes are emitted per 15-second window.
func (sampler *timingSampler) observe(sample [7]byte, now time.Time) {
	if sampler.emit == nil {
		return
	}
	if sampler.started.IsZero() {
		sampler.started = now
	}
	for stage, bucket := range sample {
		if bucket >= 1 && bucket <= 5 && bucket > sampler.maximum[stage] {
			sampler.maximum[stage] = bucket
		}
	}
	if !now.Before(sampler.started.Add(15 * time.Second)) {
		sampler.flush(now)
	}
}

func (sampler *timingSampler) flush(now time.Time) {
	if sampler.emit != nil {
		for stage, bucket := range sampler.maximum {
			if code := timingCode(stage, bucket); code != "" {
				if sampler.heartbeat {
					code = "installed_input_heartbeat_timing_" + code[len("installed_input_timing_"):]
				}
				emitTiming(sampler.emit, code)
			}
		}
	}
	sampler.maximum = [7]byte{}
	sampler.started = now
}

func emitTiming(emit func(string), code string) {
	// Diagnostics must not turn an acknowledged input into an apparent failure.
	defer func() { _ = recover() }()
	emit(code)
}

type workerTiming struct{ buckets [3]byte }

func measureExecution(authorize func() error, execute executor, timing *workerTiming) executor {
	return func(ctx context.Context, work func() error) error {
		timing.buckets = [3]byte{}
		start := time.Now()
		err := authorize()
		timing.buckets[0] = timingBucket(time.Since(start))
		if err != nil {
			return err
		}
		var applied time.Duration
		start = time.Now()
		err = execute(ctx, func() error {
			begin := time.Now()
			result := work()
			applied = time.Since(begin)
			timing.buckets[2] = timingBucket(applied)
			return result
		})
		timing.buckets[1] = timingBucket(time.Since(start) - applied)
		return err
	}
}
