package logindiag

import "testing"

func TestSourceFailureRejectsPrivateOperationsAndContradictoryNativeEvidence(t *testing.T) {
	for _, bad := range []SourceFailure{
		{Operation: "SECRET/PIN/input"},
		{Operation: "source-reader-timeout", Win32Error: 170},
		{Operation: "source-reader-timeout", Win32Known: true},
	} {
		if bad.Valid() {
			t.Fatal("unbounded or contradictory source evidence accepted", bad)
		}
	}
	for _, good := range []SourceFailure{
		{},
		{Operation: "source-reader-timeout"},
		{Operation: "source-reader-timeout", Win32Error: 170, Win32Known: true},
	} {
		if !good.Valid() {
			t.Fatal("known/unknown source evidence rejected", good)
		}
	}
}
