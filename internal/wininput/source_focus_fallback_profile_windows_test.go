//go:build windows

package wininput

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestNativeDeactivationCarriesFallbackCauseBeforeSavedFocusIsCleared(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(10)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("restore rejection") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureProbeForeground = func(uintptr) error { return errors.New("probe timeout") }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("rejected probe must not attach threads"); return nil }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) { foreground = 0 }
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, commands: make(chan sourceWheelCaptureCommand, 1)}
	current.active.Store(true)
	result := make(chan error, 1)
	current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureDeactivate, result: result, profile: p}
	current.handleCommand(wmSourceWheelCaptureDeactivate)
	if err := <-result; err != nil || current.savedFocus != 0 || current.active.Load() {
		t.Fatal("fallback diagnostics changed cleanup", err)
	}
	handoff.Finish(p, "ok")
	detail := summary.Focus
	if !summary.Valid() || detail == nil || !detail.RestoreFallbackMeasured || detail.RestoreFallback != 2 || detail.RestoreFallbackStage != handoff.FocusFallbackProbeFailed || detail.Restore != 3 || detail.PostHide != 2 || detail.ResultMeasured != 3 {
		t.Fatal("native owner lost restore fallback cause", detail)
	}
}

func TestRestoreFallbackStageKeepsNativeCallOrderAndOutcome(t *testing.T) {
	for _, test := range []struct {
		name   string
		result uint8
		stage  handoff.FocusFallbackStage
	}{
		{"success", 1, handoff.FocusFallbackSucceeded},
		{"thread", 2, handoff.FocusFallbackThreadUnavailable},
		{"probe", 2, handoff.FocusFallbackProbeFailed},
		{"attach", 2, handoff.FocusFallbackAttachFailed},
		{"set", 4, handoff.FocusFallbackSetFailed},
		{"detach", 3, handoff.FocusFallbackDetachFailed},
		{"completion", 5, handoff.FocusFallbackCompletionProbeFailed},
		{"verification", 5, handoff.FocusFallbackVerificationLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			var previousCalls []string
			for _, profiled := range []bool{false, true} {
				restore := stubSourceWheelCaptureActivation(t)
				var calls []string
				foreground := uintptr(10)
				setCalls, probeCalls := 0, 0
				cause := errors.New("synthetic native rejection")
				sourceWheelCaptureGetForeground = func() uintptr { calls = append(calls, "get"); return foreground }
				sourceWheelCaptureCurrentThread = func() uint32 { calls = append(calls, "current-thread"); return 11 }
				sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) {
					calls = append(calls, "window-thread")
					if test.name == "thread" {
						return 0, cause
					}
					return 22, nil
				}
				sourceWheelCaptureSetForeground = func(window uintptr) error {
					calls = append(calls, "set")
					setCalls++
					if setCalls == 1 || test.name == "set" {
						return cause
					}
					if test.name != "verification" {
						foreground = window
					}
					return nil
				}
				sourceWheelCaptureProbeForeground = func(uintptr) error {
					calls = append(calls, "probe")
					probeCalls++
					if test.name == "probe" || (test.name == "completion" && probeCalls == 2) {
						return cause
					}
					return nil
				}
				sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
					if attach {
						calls = append(calls, "attach")
					} else {
						calls = append(calls, "detach")
					}
					if (attach && test.name == "attach") || (!attach && test.name == "detach") {
						return cause
					}
					if !attach && test.name == "completion" {
						foreground = 10
					}
					return nil
				}
				var summary handoff.Summary
				c := handoff.New(func(s handoff.Summary) { summary = s })
				ctx := handoff.WithSession(context.Background(), c)
				p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
				current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
				profile := context.Context(nil)
				if profiled {
					profile = p
				}
				result, err := current.restoreFocusProfile(profile)
				handoff.Finish(p, handoff.Outcome(err))
				restore()
				wantRestore := SourceFocusRestoreAPIRejected
				if test.result == 1 {
					wantRestore = SourceFocusRestoreSucceeded
				}
				if result != wantRestore || (err == nil) != (test.result == 1) || (err != nil && !errors.Is(err, cause) && test.name != "verification") {
					t.Fatal("diagnostics changed restore outcome", result, err)
				}
				if !profiled {
					if summary.Focus != nil {
						t.Fatal("legacy path emitted fallback detail")
					}
					previousCalls = calls
				} else {
					wantCalls := slices.Clone(previousCalls)
					if test.name == "set" {
						index := slices.Index(wantCalls, "detach") - 1 // cleanup checks current thread before detach
						wantCalls = slices.Insert(wantCalls, index, "get")
					}
					if !slices.Equal(calls, wantCalls) || !summary.Valid() || summary.Focus == nil || !summary.Focus.RestoreFallbackMeasured || summary.Focus.RestoreFallback != test.result || summary.Focus.RestoreFallbackStage != test.stage {
						t.Fatal("unexpected native calls or lost failure stage", calls, wantCalls, summary.Focus)
					}
					if summary.Focus.RestoreRejectedForegroundMeasured != (test.name == "set") || (test.name == "set" && summary.Focus.RestoreRejectedForeground != 2) {
						t.Fatal("rejection observation missing or on wrong exit", summary.Focus)
					}
				}
			}
		})
	}
}

func TestRestoreRejectedForegroundStaysBeforeDetachAndDoesNotOverrideFailure(t *testing.T) {
	for _, detachFails := range []bool{false, true} {
		restore := stubSourceWheelCaptureActivation(t)
		foreground := uintptr(10)
		sourceWheelCaptureGetForeground = func() uintptr { return foreground }
		sourceWheelCaptureSetForeground = func(window uintptr) error {
			foreground = window
			return errors.New("rejected despite target observation")
		}
		sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
		sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
		sourceWheelCaptureProbeForeground = func(uintptr) error { return nil }
		sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
			if !attach {
				foreground = 10
				if detachFails {
					return errors.New("detach rejection")
				}
			}
			return nil
		}
		var summary handoff.Summary
		c := handoff.New(func(s handoff.Summary) { summary = s })
		p := handoff.Begin(handoff.WithSession(context.Background(), c), "source", "session", "epoch", time.Now())
		current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
		result, err := current.restoreFocusProfile(p)
		current.observeFocus(p, handoff.FocusAfterRestore)
		handoff.Finish(p, "error")
		restore()
		wantStage := handoff.FocusFallbackSetFailed
		if detachFails {
			wantStage = handoff.FocusFallbackDetachFailed
		}
		if err == nil || result != SourceFocusRestoreAPIRejected || !summary.Valid() || summary.Focus == nil || summary.Focus.RestoreFallbackStage != wantStage || summary.Focus.RestoreRejectedForeground != 3 || !summary.Focus.RestoreRejectedForegroundMeasured || summary.Focus.States[handoff.FocusAfterRestore] != 2 {
			t.Fatal("observation changed failure or crossed detach boundary", result, err, summary.Focus)
		}
	}
}

func TestRejectedForegroundQueryRequiresLiveSourceProfile(t *testing.T) {
	for _, mode := range []string{"nil", "receiver", "retired", "quota", "ended", "source"} {
		t.Run(mode, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			queries := 0
			sourceWheelCaptureGetForeground = func() uintptr { queries++; return 0 }
			sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("set rejection") }
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			sourceWheelCaptureProbeForeground = func(uintptr) error { return nil }
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return nil }
			c := handoff.New(nil)
			ctx := handoff.WithSession(context.Background(), c)
			role := "source"
			if mode == "receiver" {
				role = "receiver"
			}
			p := handoff.Begin(ctx, role, "session", "epoch", time.Now())
			switch mode {
			case "nil":
				p = nil
			case "retired":
				handoff.Finish(p, "ok")
			case "ended":
				handoff.EndSession(ctx)
			case "quota":
				for i := 1; i < handoff.Limit; i++ {
					handoff.Finish(p, "ok")
					p = handoff.Begin(ctx, role, "session", "epoch", time.Now())
				}
				handoff.Finish(p, "ok")
				p = handoff.Begin(ctx, role, "session", "epoch", time.Now())
			}
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			result, err := current.restoreFocusFallbackProfile(p)
			wantQueries := 0
			if mode == "source" {
				wantQueries = 1
			}
			if result != SourceFocusRestoreFallbackAPIRejected || err == nil || queries != wantQueries || current.attachedSource != 0 {
				t.Fatal("optional observation affected query bound or cleanup", result, err, queries)
			}
		})
	}
}

func TestRejectedForegroundRetirementDuringDetachCannotEmitPartialEvidence(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	sourceWheelCaptureGetForeground = func() uintptr { return 10 }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("set rejection") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureProbeForeground = func(uintptr) error { return nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if !attach {
			handoff.EndSession(ctx)
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	result, err := current.restoreFocusFallbackProfile(p)
	if result != SourceFocusRestoreFallbackAPIRejected || err == nil || !summary.Valid() || summary.Focus != nil || current.attachedSource != 0 {
		t.Fatal("retirement emitted partial rejection evidence or changed cleanup", result, err, summary.Focus)
	}
}
