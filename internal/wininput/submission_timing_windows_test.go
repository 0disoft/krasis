//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

func TestSendInputTimingKeepsSingleCallAndOutcome(t *testing.T) {
	for _, mode := range []string{"ok", "partial", "rejected", "unknown", "cancelled", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				original := callSendInput
				t.Cleanup(func() { callSendInput = original })
				calls := 0
				callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
					calls++
					time.Sleep(18*time.Millisecond + 602*time.Microsecond)
					switch mode {
					case "partial":
						return 1, 0, errors.New("partial")
					case "rejected":
						return 0, 0, errors.New("rejected")
					case "unknown":
						return 3, 0, errors.New("unknown")
					default:
						return 2, 0, nil
					}
				}
				var p handoff.WorkerApplyProfile
				ctx := context.Background()
				if mode != "disabled" {
					ctx = handoff.WithWorkerApply(ctx, &p)
				}
				if mode == "cancelled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				err := sendInputs(ctx, []input{mouseEvent(mouseEventWheel, 120, 0, 0), mouseEvent(mouseEventHWheel, 120, 0, 0)})
				if mode == "cancelled" {
					if calls != 0 || p.SubmissionCalls != 0 || !errors.Is(err, context.Canceled) {
						t.Fatal("cancelled input submitted or timed")
					}
					return
				}
				if calls != 1 {
					t.Fatal("native input replayed")
				}
				if mode == "disabled" {
					if p.SubmissionCalls != 0 {
						t.Fatal("disabled input timed")
					}
				} else if p.SubmissionCalls != 1 || p.SubmissionMicros != 18602 {
					t.Fatal("native call duration not isolated")
				}
				certainty, classified := application.InjectionCertaintyOf(err)
				expected := map[string]application.InjectionCertainty{"partial": application.InjectionPartiallyApplied, "rejected": application.InjectionNotApplied, "unknown": application.InjectionOutcomeUnknown}
				if want, failed := expected[mode]; failed {
					if !classified || certainty != want {
						t.Fatal("timing changed outcome certainty")
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
