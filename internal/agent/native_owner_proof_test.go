package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/cleanupresult"
	"github.com/0disoft/krasis/internal/control"
)

func TestSourceHookRetirementProofPreservesFailureAndRejectsFailedCleanup(t *testing.T) {
	native := errors.New("native query failed")
	cleanup := errors.New("native close failed")
	for _, tc := range []struct {
		name    string
		outcome error
		retired bool
	}{
		{"verified", cleanupresult.Confirm(native), true},
		{"unverified", native, false},
		{"additional close failure", errors.Join(cleanupresult.Confirm(native), cleanup), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right := openConnectionPair(t)
			defer right.Close()
			ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			result, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10}), Dependencies{
				Connection: left, Hook: contextErrorHook{err: tc.outcome}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
			})
			if !errors.Is(err, native) || errors.Is(err, ErrRecoveryUnverified) == tc.retired {
				t.Fatal("lost native failure or cleanup uncertainty", err)
			}
			if tc.retired && (result.FinalState != control.StateLocal || result.Recovery.NativeOwnersReleased != RecoveryConfirmed || result.Recovery.PressedLedgerEmpty != RecoveryConfirmed) {
				t.Fatal("unconfirmed native retirement", result)
			}
			if tc.name == "additional close failure" && !errors.Is(err, cleanup) {
				t.Fatal("lost close failure", err)
			}
		})
	}
}
