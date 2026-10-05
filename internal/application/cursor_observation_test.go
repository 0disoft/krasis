package application

import "testing"

func TestCursorObservationPresenceAndInvalidCombinations(t *testing.T) {
	p := CursorPlacementObservation{}
	p.Record(CursorTargetAdjusted, false)
	p.Record(CursorReadbackMatches, false)
	p.Record(CursorClipChanged, false)
	if !p.Valid() || p.Known != 7 || p.Flags != 0 {
		t.Fatal("false versus absent lost")
	}
	p.Record(CursorClipChanged, true)
	if p.Flags != 0 {
		t.Fatal("observation overwritten")
	}
	for _, bad := range []CursorPlacementObservation{
		{}, {Known: 1}, {Known: 131}, {Known: 3, Flags: 4},
		{Known: 19}, {Known: 7, Flags: CursorReadbackMatches},
		{Known: 51, Flags: CursorPhysicalMatchesTarget | CursorPhysicalMatchesReadback},
	} {
		if bad.Valid() {
			t.Fatal("invalid observation", bad)
		}
	}
	if !(CursorPlacementObservation{Known: 3, Flags: CursorReadbackMatches}).Valid() {
		t.Fatal("matching receipt rejected")
	}
}
