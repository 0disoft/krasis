package handoff

import (
	"context"
	"time"
)

type workerApplyKey struct{}

// WithWorkerApply binds a synchronous native call to its containing worker
// apply. The worker invokes this only for an already bounded diagnostic request.
// It must not escape to asynchronous work or subsequent cursor/recovery calls.
func WithWorkerApply(ctx context.Context, p *WorkerApplyProfile) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, workerApplyKey{}, p)
}

func WorkerApplyProbe(ctx context.Context) *WorkerApplyProfile {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(workerApplyKey{}).(*WorkerApplyProfile)
	return p
}

// RecordSubmission measures exactly one native SendInput call. Current input
// operations submit at most once. If that changes, do not silently select one
// call or sum overlapping observations: invalidate metadata without retrying.
func (p *WorkerApplyProfile) RecordSubmission(elapsed time.Duration) {
	if p == nil {
		return
	}
	if p.SubmissionCalls != 0 {
		p.SubmissionCalls = 2
		return
	}
	p.SubmissionCalls = 1
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
		p.SubmissionSaturated = true
	}
	p.SubmissionMicros = uint32(elapsed.Microseconds())
}
