//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

func TestConfinedCursorTargetRespectsExclusiveNegativeAndDisjointBounds(t *testing.T) {
	for _, test := range []struct {
		x, y int32
		clip cursorBounds
		want point
		bad  bool
	}{
		{-100, 10, cursorBounds{-200, -50, 0, 50}, point{-100, 10}, false},
		{-500, 100, cursorBounds{-200, -50, 0, 50}, point{-200, 49}, false},
		{100, -100, cursorBounds{-200, -50, 0, 50}, point{-1, -50}, false},
		{0, 0, cursorBounds{20, 20, 21, 21}, point{20, 20}, false},
		{0, 0, cursorBounds{20, 20, 20, 21}, point{}, true},
		{0, 0, cursorBounds{1000, 0, 1100, 100}, point{}, true},
		{-1001, 0, cursorBounds{-1000, -500, 1000, 500}, point{}, true},
	} {
		got, err := confinedCursorTarget(test.x, test.y, test.clip, -1000, -500, 2000, 1000)
		if (err != nil) != test.bad || !test.bad && got != test.want {
			t.Fatalf("confined target = %+v, error %v", got, err)
		}
	}
}

func TestNativePlacementSetsOnceAndPreservesUnknownReadback(t *testing.T) {
	oldClip, oldSet, oldGet := readCursorBounds, callSetCursorPos, callGetCursorPos
	t.Cleanup(func() { readCursorBounds, callSetCursorPos, callGetCursorPos = oldClip, oldSet, oldGet })
	left, top := systemMetric(smXVIRTUALSCREEN), systemMetric(smYVIRTUALSCREEN)
	if systemMetric(smCXVIRTUALSCREEN) < 10 || systemMetric(smCYVIRTUALSCREEN) < 10 {
		t.Skip("no usable test screen geometry")
	}
	for _, mode := range []string{"clipped", "mismatch", "clip_failed", "set_failed", "read_failed", "cancel_before", "cancel_after"} {
		t.Run(mode, func(t *testing.T) {
			sets, reads := 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			readCursorBounds = func(b *cursorBounds) bool {
				*b = cursorBounds{left + 3, top + 3, left + 6, top + 6}
				return mode != "clip_failed"
			}
			callSetCursorPos = func(args ...uintptr) (uintptr, uintptr, error) {
				sets++
				if int32(args[0]) != left+3 || int32(args[1]) != top+3 {
					t.Fatal("target did not respect observed confinement")
				}
				if mode == "cancel_after" {
					cancel()
				}
				if mode == "set_failed" {
					return 0, 0, errors.New("set failure")
				}
				return 1, 0, nil
			}
			callGetCursorPos = func(p *point) (uintptr, uintptr, error) {
				reads++
				*p = point{left + 3, top + 3}
				if mode == "mismatch" {
					p.X++
				}
				if mode == "read_failed" {
					return 0, 0, errors.New("read failure")
				}
				return 1, 0, nil
			}
			if mode == "cancel_before" {
				cancel()
			}
			p, err := (nativeInjectionPort{}).PlaceCursor(ctx, left+1, top+1)
			if mode == "clipped" || mode == "mismatch" {
				if err != nil || sets != 1 || reads != 1 || p.TargetX != left+3 || p.TargetY != top+3 || (p.X != p.TargetX) != (mode == "mismatch") {
					t.Fatal("placement receipt changed observed position", err)
				}
				return
			}
			certainty, known := application.InjectionCertaintyOf(err)
			want := application.InjectionOutcomeUnknown
			if mode == "clip_failed" || mode == "cancel_before" {
				want = application.InjectionNotApplied
				if sets != 0 || reads != 0 {
					t.Fatal("unapplied placement touched cursor")
				}
			} else if sets != 1 {
				t.Fatal("cursor set replayed")
			}
			if !known || certainty != want {
				t.Fatal("placement failure lost certainty", err)
			}
		})
	}
}
