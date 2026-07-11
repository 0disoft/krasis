package control

import (
	"errors"
	"reflect"
	"testing"
)

func TestPressedStatePlansReversePressOrder(t *testing.T) {
	var state PressedState
	first := PressedToken{Kind: PressedKeyboardKey, Code: 17}
	second := PressedToken{Kind: PressedKeyboardKey, Code: 65}
	third := PressedToken{Kind: PressedMouseButton, Code: 0}
	for _, token := range []PressedToken{first, second, third} {
		added, err := state.RecordPress(token)
		if err != nil || !added {
			t.Fatalf("RecordPress(%+v) = added %t, error %v", token, added, err)
		}
	}

	plan := state.ReleasePlan()
	want := []PressedToken{third, second, first}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("release plan = %v, want %v", plan, want)
	}
}

func TestPressedStateKeepsTokenUntilSuccessfulReleaseIsRecorded(t *testing.T) {
	var state PressedState
	token := PressedToken{Kind: PressedKeyboardKey, Code: 16}
	if _, err := state.RecordPress(token); err != nil {
		t.Fatalf("RecordPress returned error: %v", err)
	}
	if !state.Contains(token) || state.Count() != 1 {
		t.Fatalf("pressed token was not retained: count=%d", state.Count())
	}

	released, err := state.RecordRelease(token)
	if err != nil || !released {
		t.Fatalf("RecordRelease = released %t, error %v", released, err)
	}
	if state.Contains(token) || state.Count() != 0 || len(state.ReleasePlan()) != 0 {
		t.Fatalf("released token remains tracked: count=%d plan=%v", state.Count(), state.ReleasePlan())
	}
}

func TestPressedStateMakesRepeatedDownAndUpIdempotent(t *testing.T) {
	var state PressedState
	token := PressedToken{Kind: PressedMouseButton, Code: 1}
	added, err := state.RecordPress(token)
	if err != nil || !added {
		t.Fatalf("first RecordPress = added %t, error %v", added, err)
	}
	added, err = state.RecordPress(token)
	if err != nil || added || state.Count() != 1 {
		t.Fatalf("duplicate RecordPress = added %t, count %d, error %v", added, state.Count(), err)
	}
	if _, err := state.RecordRelease(token); err != nil {
		t.Fatalf("first RecordRelease returned error: %v", err)
	}
	released, err := state.RecordRelease(token)
	if err != nil || released {
		t.Fatalf("duplicate RecordRelease = released %t, error %v", released, err)
	}
}

func TestPressedStateReleasePlanDoesNotExposeInternalSlice(t *testing.T) {
	var state PressedState
	token := PressedToken{Kind: PressedKeyboardKey, Code: 65}
	if _, err := state.RecordPress(token); err != nil {
		t.Fatalf("RecordPress returned error: %v", err)
	}
	plan := state.ReleasePlan()
	plan[0] = PressedToken{Kind: PressedMouseButton, Code: 99}
	if !state.Contains(token) || state.ReleasePlan()[0] != token {
		t.Fatal("caller mutated PressedState through ReleasePlan")
	}
}

func TestPressedStateRejectsUnknownKind(t *testing.T) {
	var state PressedState
	_, err := state.RecordPress(PressedToken{Kind: "TOUCH_CONTACT", Code: 1})
	if !errors.Is(err, ErrInvalidPressedToken) {
		t.Fatalf("error = %v, want ErrInvalidPressedToken", err)
	}
	if state.Count() != 0 {
		t.Fatalf("invalid token changed state: count=%d", state.Count())
	}
}
