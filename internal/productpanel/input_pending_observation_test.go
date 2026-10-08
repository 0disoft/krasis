package productpanel

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPendingRestorationRetainsReadOnlyConfirmationAfterTransientFailures(t *testing.T) {
	for _, mode := range []string{"status", "trust", "before send", "after send", "expired", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			b, c, now := newResumeTest(t, nil)
			b.updatedResume = nil
			c.state = "CONNECTED"
			b.RefreshProtectedInput(func(context.Context) error { return &inputMaintenancePending{} })
			b.PendingInputSetup = func() string { return "READY" }
			restores, promotions := 0, 0
			b.StartupReady = func() { promotions++ }
			b.PendingInputRestore = func(context.Context) error {
				restores++
				if mode == "trust" {
					c.trustErr = errors.New("temporary trust failure")
				} else if mode != "before send" && mode != "after send" {
					c.err = errors.New("temporary status failure")
				}
				return nil
			}
			if mode == "before send" || mode == "after send" {
				staged := &stagedResumeClient{resumeTestClient: c}
				// Tick + confirmation + command preflight, then post-send status.
				staged.failStatus = 3
				if mode == "after send" {
					staged.failStatus = 4
				}
				b.open = func() (Client, error) { return staged, nil }
			}
			b.tick(now)
			if restores != 1 || promotions != 1 {
				t.Fatal("restoration not completed once", restores, promotions)
			}
			if mode != "after send" && (b.pendingInput == nil || !b.pendingInput.restored || c.connects != 0) {
				t.Fatal("lost read-only confirmation", b.Refresh(), c.connects)
			}
			if mode == "expired" {
				b.pendingInput.deadline = now
			}
			if mode == "disconnect" {
				b.Disconnect()
			}
			c.err, c.trustErr = nil, nil
			if mode == "after send" {
				c.state = "LOCAL"
			}
			b.tick(now.Add(time.Second))
			b.tick(now.Add(2 * time.Second))
			want := 1
			if mode == "expired" || mode == "disconnect" {
				want = 0
			}
			if restores != 1 || promotions != 1 || c.connects != want || b.pendingInput != nil {
				t.Fatal("restoration replayed or pending connection lost", mode, b.Refresh(), restores, promotions, c.connects)
			}
		})
	}
}
