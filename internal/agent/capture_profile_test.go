package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type profiledTestCapture struct{ recordingActivation }

func (capture *profiledTestCapture) ActivateProfile(ctx context.Context) error {
	handoff.RecordCapture(ctx, handoff.CaptureTotal, time.Millisecond)
	return capture.recordingActivation.Activate()
}

func (capture *profiledTestCapture) DeactivateProfile(ctx context.Context) {
	handoff.RecordFocusResult(ctx, 0, 2)
	capture.recordingActivation.Deactivate()
}

func TestCombinedDeactivationKeepsActivationAttemptAfterCurrentChanges(t *testing.T) {
	var summaries []handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "first", "epoch", time.Now())
	var order []string
	activation := &combinedSourceActivation{
		profile: ctx,
		source:  &recordingActivation{name: "source", order: &order},
		capture: &profiledTestCapture{recordingActivation{name: "capture", order: &order}},
	}
	if err := activation.Activate(); err != nil {
		t.Fatal(err)
	}
	next := handoff.Begin(ctx, "source", "next", "epoch", time.Now())
	activation.Deactivate()
	handoff.Finish(p, "ok")
	handoff.Finish(next, "ok")
	if len(summaries) != 2 || summaries[0].Focus != nil || summaries[1].Focus != nil {
		t.Fatal("deactivation attached retired capture to current attempt", summaries)
	}
}

func TestCombinedFailedActivationProfilesCleanupBeforeRetire(t *testing.T) {
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "first", "epoch", time.Now())
	var order []string
	activation := &combinedSourceActivation{
		profile: ctx,
		source:  &recordingActivation{name: "source", order: &order},
		capture: &profiledTestCapture{recordingActivation{name: "capture", order: &order, activateErr: errors.New("capture failure")}},
	}
	if err := activation.Activate(); err == nil {
		t.Fatal("capture failure changed to success")
	}
	handoff.Finish(p, "error")
	if !summary.Valid() || summary.Focus == nil || summary.Focus.Restore != 2 {
		t.Fatal("failed activation lost original cleanup context", summary)
	}
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
