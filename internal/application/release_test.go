package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

var errReleaseBlocked = errors.New("release blocked")

type recordingReleasePort struct {
	fail  map[control.PressedToken]error
	calls []control.PressedToken
}

func (port *recordingReleasePort) Release(_ context.Context, token control.PressedToken) error {
	port.calls = append(port.calls, token)
	return port.fail[token]
}

func TestReleaseAllReleasesInReverseOrderAndClearsState(t *testing.T) {
	var state control.PressedState
	first := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 17}
	second := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	third := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}
	pressAll(t, &state, first, second, third)
	port := &recordingReleasePort{}

	report, err := ReleaseAll(context.Background(), &state, port)
	if err != nil {
		t.Fatalf("ReleaseAll returned error: %v", err)
	}
	if report.Attempted != 3 || report.Released != 3 || report.Remaining != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if want := []control.PressedToken{third, second, first}; !reflect.DeepEqual(port.calls, want) {
		t.Fatalf("release order = %v, want %v", port.calls, want)
	}
}

func TestReleaseAllContinuesAfterFailureAndRetainsOnlyFailedTokens(t *testing.T) {
	var state control.PressedState
	first := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 17}
	second := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	third := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}
	pressAll(t, &state, first, second, third)
	port := &recordingReleasePort{fail: map[control.PressedToken]error{second: errReleaseBlocked}}

	report, err := ReleaseAll(context.Background(), &state, port)
	if !errors.Is(err, errReleaseBlocked) {
		t.Fatalf("error = %v, want release failure", err)
	}
	if report.Attempted != 3 || report.Released != 2 || report.Remaining != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !state.Contains(second) || state.Contains(first) || state.Contains(third) {
		t.Fatalf("unexpected remaining pressed state: %v", state.ReleasePlan())
	}
	if len(port.calls) != 3 {
		t.Fatalf("ReleaseAll stopped after a failure: calls=%v", port.calls)
	}

	port.fail = nil
	retry, err := ReleaseAll(context.Background(), &state, port)
	if err != nil || retry.Attempted != 1 || retry.Released != 1 || retry.Remaining != 0 {
		t.Fatalf("retry = %+v, error %v", retry, err)
	}
}

func TestReleaseAllIsIdempotentWhenNothingIsPressed(t *testing.T) {
	var state control.PressedState
	port := &recordingReleasePort{}

	report, err := ReleaseAll(context.Background(), &state, port)
	if err != nil || report != (ReleaseReport{}) || len(port.calls) != 0 {
		t.Fatalf("empty ReleaseAll = report %+v, calls %v, error %v", report, port.calls, err)
	}
}

func pressAll(t *testing.T, state *control.PressedState, tokens ...control.PressedToken) {
	t.Helper()
	for _, token := range tokens {
		if _, err := state.RecordPress(token); err != nil {
			t.Fatalf("RecordPress(%+v) returned error: %v", token, err)
		}
	}
}
