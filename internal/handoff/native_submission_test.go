package handoff

import (
	"context"
	"testing"
	"time"
)

func TestNativeSubmissionRequiresMeasuredParentAndSingleCall(t *testing.T) {
	var p WorkerApplyProfile
	if WorkerApplyProbe(nil) != nil || WorkerApplyProbe(context.Background()) != nil {
		t.Fatal("untracked native timing enabled")
	}
	ctx := WithWorkerApply(context.Background(), &p)
	if WorkerApplyProbe(ctx) != &p {
		t.Fatal("native receipt lost operation identity")
	}
	p.RecordSubmission(18*time.Millisecond + 602*time.Microsecond)
	if p.Valid() {
		t.Fatal("submission accepted without containing apply")
	}
	p.Record(0, 19*time.Millisecond)
	if !p.Valid() || p.SubmissionMicros != 18602 || p.SubmissionCalls != 1 {
		t.Fatal("nested submission lost")
	}
	p.Micros[0] = 18601
	if p.Valid() {
		t.Fatal("native duration escaped parent")
	}
	p.Micros[0] = 19000
	p.RecordSubmission(time.Millisecond)
	if p.Valid() || p.SubmissionMicros != 18602 || p.SubmissionCalls != 2 {
		t.Fatal("multiple submissions silently selected or summed")
	}
	for _, duration := range []time.Duration{0, -1, Window + time.Second} {
		var p WorkerApplyProfile
		p.RecordSubmission(duration)
		p.Record(0, duration)
		if !p.Valid() || p.SubmissionCalls != 1 || p.SubmissionSaturated != (duration > Window) {
			t.Fatal("native presence or saturation lost")
		}
	}
}
