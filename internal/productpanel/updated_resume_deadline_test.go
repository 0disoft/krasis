package productpanel

import (
	"context"
	"testing"
	"time"
)

func TestUpdateConfirmationExpiryDoesNotFaultActiveConnection(t *testing.T) {
	for _, state := range []string{"CONNECTING", "RECONNECTING"} {
		t.Run(state, func(t *testing.T) {
			b, c, now := newResumeTest(t, nil)
			b.tick(now)
			c.state = state
			b.tick(now.Add(91 * time.Second))
			b.tick(now.Add(100 * time.Second))
			if b.updatedResume != nil || b.Refresh().Code != "ok" || b.Refresh().State != state || c.connects != 1 {
				t.Fatal("healthy supervisor mislabeled or command replayed", b.Refresh(), c.connects)
			}
			c.state = "CONNECTED"
			b.tick(now.Add(110 * time.Second))
			if b.Refresh().Code != "ok" || b.recovery.Alias != "B" || b.recovery.Fingerprint != c.peers[0].Fingerprint || c.connects != 1 {
				t.Fatal("late connection did not become ordinary recovery", b.Refresh())
			}
		})
	}
}

func TestUpdateConfirmationExpiryKeepsUncertainCommandFence(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	c.uncertain = true
	b.tick(now)
	b.tick(now.Add(91 * time.Second))
	b.tick(now.Add(100 * time.Second))
	if b.updatedResume != nil || b.Refresh().Code != "outcome_unknown" || !b.recovery.uncertain || c.connects != 1 {
		t.Fatal("unknown command became a recovery failure or replay", b.Refresh(), c.connects)
	}
	c.state = "FAULTED"
	b.tick(now.Add(110 * time.Second))
	if c.connects != 1 || b.Refresh().Code != "outcome_unknown" {
		t.Fatal("ordinary recovery replayed uncertain update command", b.Refresh(), c.connects)
	}
	c.state = "CONNECTED"
	b.tick(now.Add(120 * time.Second))
	if b.recovery.uncertain || b.Refresh().Code != "ok" || c.connects != 1 {
		t.Fatal("pinned connected observation did not resolve uncertainty", b.Refresh())
	}
}

func TestRecoveryFailureNoticeClearsOnlyForHealthyActiveObservation(t *testing.T) {
	for _, state := range []string{"LOCAL", "FAULTED", "CONNECTING", "RECONNECTING", "CONNECTED"} {
		t.Run(state, func(t *testing.T) {
			b, c, now := newResumeTest(t, func(context.Context) error { t.Fatal("notice refresh restarted agent"); return nil })
			b.updatedResume = nil
			c.state = state
			b.recovery.Attempts = 3
			b.recovery.diagnosed = true
			old := NewDesktopBridge(context.Background(), b.open).Refresh()
			old.Code = "recovery_exhausted"
			b.save(old)
			b.tick(now)
			want := "ok"
			if state == "LOCAL" || state == "FAULTED" {
				want = "recovery_exhausted"
			}
			if b.Refresh().Code != want || c.connects != 0 {
				t.Fatal(state, b.Refresh(), c.connects)
			}
		})
	}
}
