//go:build windows

package wininput

import (
	"errors"
	"testing"
	"unsafe"
)

func TestInputStructMatchesWindowsABI(t *testing.T) {
	want := uintptr(28)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 40
	}
	if got := unsafe.Sizeof(input{}); got != want {
		t.Fatalf("INPUT size = %d, want %d", got, want)
	}
}

func TestKeyboardInputStructMatchesWindowsABI(t *testing.T) {
	want := uintptr(16)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 24
	}
	if got := unsafe.Sizeof(keyboardInput{}); got != want {
		t.Fatalf("KEYBDINPUT size = %d, want %d", got, want)
	}
}

func TestClassifySelfTest(t *testing.T) {
	tests := []struct {
		name          string
		outcome       injectionOutcome
		observed      uint64
		injectedMoves uint64
		physicalMoves uint64
		passed        bool
		failureCode   string
	}{
		{name: "send failure", outcome: injectionOutcome{err: errors.New("blocked")}, failureCode: "send_input_failed"},
		{name: "partial send", outcome: injectionOutcome{sent: 1, pointerRestored: true}, observed: 1, failureCode: "send_input_incomplete"},
		{name: "not observed", outcome: injectionOutcome{sent: 2, pointerRestored: true}, observed: 1, failureCode: "injected_events_not_observed"},
		{name: "physical interference", outcome: injectionOutcome{sent: 2}, observed: 2, physicalMoves: 1, failureCode: "physical_input_interference"},
		{name: "pointer changed", outcome: injectionOutcome{sent: 2}, observed: 2, failureCode: "pointer_position_changed"},
		{name: "pass", outcome: injectionOutcome{sent: 2, pointerRestored: true}, observed: 2, passed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := classifySelfTest(test.outcome, test.observed, test.injectedMoves, test.physicalMoves)
			if result.Passed != test.passed || result.FailureCode != test.failureCode {
				t.Fatalf("result = %+v, want passed=%t failureCode=%q", result, test.passed, test.failureCode)
			}
		})
	}
}
