package handoff

import "testing"

func TestPreparationDecisionPreservesLegacyAndRejectsContradictoryEvidence(t *testing.T) {
	for decision := PreparationUnobserved; decision <= PreparationInvalidAge; decision++ {
		p := StartupProfile{Schema: 1, Measured: 3, PreparationDecision: decision, Prepared: decision == PreparationUsed}
		if !p.Valid() {
			t.Fatal("known preparation decision rejected", decision)
		}
		if decision != PreparationUnobserved {
			p.Prepared = !p.Prepared
			if p.Valid() {
				t.Fatal("contradictory prepared marker accepted", decision)
			}
		}
	}
	for _, p := range []StartupProfile{
		{Schema: 1, Measured: 3, PreparationDecision: PreparationInvalidAge + 1},
		{Schema: 1, Measured: 1, PreparationDecision: PreparationNoCandidate},
		{Schema: 1, Measured: 3, PreparationDecision: PreparationExpired, PreparedAgeMicros: 1},
	} {
		if p.Valid() {
			t.Fatal("unbounded or unmeasured preparation decision accepted")
		}
	}
	if !(StartupProfile{Schema: 1, Measured: 3, Prepared: true}).Valid() {
		t.Fatal("legacy prepared evidence rejected")
	}
}
