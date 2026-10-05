//go:build windows

package inputworker

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPreparedSelectionSeparatesExpiryFromRetirementWithoutInput(t *testing.T) {
	for _, expired := range []bool{false, true} {
		p := &preparedConsole{warmTimer: time.NewTimer(time.Hour)}
		defer p.warmTimer.Stop()
		want := handoff.PreparationClaimLost
		if expired {
			if p.expireUnbound() != nil {
				t.Fatal("unbound expiration failed")
			}
			want = handoff.PreparationExpired
		} else if p.retireUnbound() != nil {
			t.Fatal("unbound retirement failed")
		}
		if !p.retired() || p.claimDecision(context.Background()) != want || p.warmState.Load() < 2 || p.profile.Prepared {
			t.Fatal("selection confused expiration and explicit retirement")
		}
	}
}

func TestPreparedSelectionNeverReusesUnusableCancelledOrClaimedCandidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if (*preparedConsole)(nil).claimDecision(context.Background()) != handoff.PreparationNoCandidate {
		t.Fatal("absent candidate did not remain absent")
	}
	p := &preparedConsole{}
	if p.claimDecision(ctx) != handoff.PreparationCancelled || p.claimDecision(nil) != handoff.PreparationCancelled || p.warmState.Load() != 0 {
		t.Fatal("cancelled claim mutated ownership")
	}
	if p.claimDecision(context.Background()) != handoff.PreparationUnusable || p.warmState.Load() != 0 {
		t.Fatal("candidate without service timer was reused")
	}
	p.warmTimer = time.NewTimer(time.Hour)
	defer p.warmTimer.Stop()
	if p.claimDecision(context.Background()) != handoff.PreparationUnusable || p.warmState.Load() != 1 || p.profile.Prepared {
		t.Fatal("missing worker resources became a successful selection")
	}
	if p.expireUnbound() != nil || p.warmState.Load() != 1 || p.claimDecision(context.Background()) != handoff.PreparationClaimLost {
		t.Fatal("expiry or repeated claim took the claimed candidate")
	}
}
