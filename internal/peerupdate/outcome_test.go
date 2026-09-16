package peerupdate

import (
	"errors"
	"testing"
)

func TestVersionOutcomesDoNotConflateDifferentBuilds(t *testing.T) {
	for _, c := range []struct {
		local, target int
		a, b          string
		want          error
	}{
		{24, 24, "a", "a", ErrSameVersion}, {24, 24, "a", "b", ErrGenerationConflict},
		{25, 24, "a", "b", ErrNewerVersion}, {23, 24, "a", "b", nil},
	} {
		if got := compareVersion(c.local, c.a, c.target, c.b); !errors.Is(got, c.want) {
			t.Fatalf("got %v want %v", got, c.want)
		}
	}
	if wireResult("private path") != "PEER_REJECTED" {
		t.Fatal("untrusted result exposed")
	}
	for _, err := range []error{ErrSameVersion, ErrNewerVersion, ErrGenerationConflict, ErrHashMismatch, ErrStorage, ErrTransfer, ErrDenied, ErrBusy} {
		if wireResult(FailureCode(err)) != FailureCode(err) {
			t.Fatal("result cannot round-trip")
		}
	}
}
