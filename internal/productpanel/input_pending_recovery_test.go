package productpanel

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestPendingInputContinuationWaitsAndResumesOnce(t *testing.T) {
	for _, mode := range []string{"ready", "mismatch", "unknown", "manual stop", "changed peer", "lost reply", "restore failed", "expired", "unverified exit", "fresh pending"} {
		t.Run(mode, func(t *testing.T) {
			b, c, now := newResumeTest(t, nil)
			b.updatedResume = nil
			c.state = "CONNECTED"
			setup, restores, promotions := "PENDING", 0, 0
			b.PendingInputSetup = func() string { return setup }
			b.StartupReady = func() { promotions++ }
			b.PendingInputRestore = func(context.Context) error {
				restores++
				if mode == "restore failed" {
					return errors.New("restore failed")
				}
				if mode == "fresh pending" {
					return ErrInputRefreshPending
				}
				return nil
			}
			b.RefreshProtectedInput(func(context.Context) error {
				if mode == "unverified exit" {
					return ErrInputRefreshPending
				}
				return &inputMaintenancePending{}
			})
			b.tick(now)
			if restores != 0 || c.connects != 0 {
				t.Fatal("continued during installation")
			}
			setup = "READY"
			if mode == "mismatch" {
				setup = "UPDATE_REQUIRED"
			}
			if mode == "unknown" {
				setup = "UNKNOWN"
			}
			if mode == "manual stop" {
				c.manualStop = true
			}
			if mode == "changed peer" {
				c.peers[0].Fingerprint = strings.Repeat("c", 64)
			}
			if mode == "lost reply" {
				c.uncertain = true
			}
			if mode == "expired" {
				b.pendingInput.deadline = now
			}
			b.tick(now.Add(time.Second))
			if mode == "lost reply" {
				c.state = "LOCAL"
			}
			for i := 2; i < 5; i++ {
				b.tick(now.Add(time.Duration(i) * time.Second))
			}
			wantRestores, wantConnects, wantPromotions := 1, 1, 1
			switch mode {
			case "unknown", "expired", "unverified exit":
				wantRestores, wantConnects, wantPromotions = 0, 0, 0
			case "manual stop", "changed peer":
				wantConnects = 0
			case "restore failed":
				wantConnects, wantPromotions = 0, 0
			case "fresh pending":
				wantRestores, wantConnects, wantPromotions = 4, 0, 0 // read-only completion check only
			}
			if restores != wantRestores || c.connects != wantConnects || promotions != wantPromotions {
				t.Fatal(mode, b.Refresh(), restores, c.connects, promotions)
			}
			if mode == "mismatch" && b.Refresh().InputRefreshCode != "input_refresh_failed" {
				t.Fatal("mismatch advertised as installed", b.Refresh())
			}
			if mode == "lost reply" && b.Refresh().Code != "outcome_unknown" {
				t.Fatal("uncertain continuation lost", b.Refresh())
			}
		})
	}
}

func TestDisconnectCancelsPendingInputContinuation(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.updatedResume = nil
	c.state = "CONNECTED"
	b.RefreshProtectedInput(func(context.Context) error { return &inputMaintenancePending{} })
	b.PendingInputSetup = func() string { return "READY" }
	b.PendingInputRestore = func(context.Context) error { t.Fatal("restored after manual stop"); return nil }
	b.Disconnect()
	b.tick(now)
	if b.pendingInput != nil || c.connects != 0 {
		t.Fatal("manual stop discarded", b.Refresh())
	}
}

func TestPendingRestorationNeverInvokesInstallerAgain(t *testing.T) {
	for _, setup := range []string{"READY", "PENDING", "UNKNOWN"} {
		t.Run(setup, func(t *testing.T) {
			missing, starts := true, 0
			r := &runtimeStub{generation: 4}
			refresh := &inputRefresh{inspect: func() string { return setup }, run: func(context.Context) error { t.Fatal("installer repeated"); return nil }}
			s := Startup{Generation: 4, Lock: func() (io.Closer, error) { return closerStub{}, nil }, Verify: func() error { return nil },
				Open: func() (RuntimeClient, error) {
					if missing {
						return nil, localapi.ErrUnavailable
					}
					return r, nil
				},
				Available: func() (bool, error) { return missing, nil }, Pause: func(context.Context) error { return nil },
				BeforeStart: refresh.automatic, InputRestoreCheck: refresh.restoreCheck,
				Start: func() error { starts++; missing = false; return nil }}
			err := s.RestoreProtectedInput(context.Background())
			if setup == "READY" && (err != nil || starts != 1) || setup != "READY" && (!errors.Is(err, ErrInputRefreshPending) || starts != 0) {
				t.Fatal(setup, err, starts)
			}
		})
	}
}

func TestPendingStartupCompletesBeforeUpdatedReconnectBudgetStarts(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.updatedResume.deadline = time.Time{}
	b.failedStartup(context.Background(), &inputMaintenancePending{})
	b.startUpdatedResumeWindow(now)
	if !b.updatedResume.deadline.IsZero() {
		t.Fatal("pending installation consumed reconnect budget")
	}
	setup, starts, loginCalls := "PENDING", 0, 0
	b.PendingInputSetup = func() string { return setup }
	b.PendingInputRestore = func(context.Context) error { starts++; return nil }
	b.StartupResume = func(_ context.Context, value BridgeSnapshot) BridgeSnapshot { loginCalls++; return value }
	b.tick(now)
	setup = "READY"
	b.tick(now.Add(time.Minute))
	if starts != 1 || loginCalls != 1 || b.pendingInput != nil || b.updatedResume.deadline != now.Add(time.Minute+90*time.Second) || c.connects != 0 {
		t.Fatal(b.Refresh(), starts, loginCalls, b.updatedResume)
	}
}
