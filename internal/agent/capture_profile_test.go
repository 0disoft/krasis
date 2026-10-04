package agent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type profiledTestCapture struct{ recordingActivation }

func (capture *profiledTestCapture) ActivateProfile(ctx context.Context) error {
	handoff.RecordCapture(ctx, handoff.CaptureTotal, time.Millisecond)
	return capture.recordingActivation.Activate()
}

func TestCombinedActivationPassesMatchingSourceProfile(t *testing.T) {
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	var order []string
	activation := &combinedSourceActivation{
		profile: ctx,
		source:  &recordingActivation{name: "source", order: &order},
		capture: &profiledTestCapture{recordingActivation{name: "capture", order: &order}},
	}
	if err := activation.Activate(); err != nil {
		t.Fatal(err)
	}
	handoff.Finish(p, "incomplete")
	if !summary.Valid() || summary.Capture == nil || summary.Capture.Micros[handoff.CaptureTotal] != 1000 {
		t.Fatal("combined activation lost original source context", summary)
	}
	activation.Deactivate()
}
