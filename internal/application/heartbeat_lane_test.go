package application

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestHeartbeatAheadRejectsExpiredForeignPendingAndCanceledLeases(t *testing.T) {
	for _, mode := range []string{"accepted", "expired", "foreign", "pending", "canceled", "backward"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime := newTestRuntime(t, &recordingInputPort{})
				if mode == "pending" {
					runtime.PrepareInbound(context.Background(), "session-1", "lease-1", "epoch-1")
				} else {
					grantInbound(t, runtime, time.Now())
				}
				before := runtime.LeaseWatch()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				observed := time.Now().Add(time.Second)
				session := "session-1"
				switch mode {
				case "expired":
					time.Sleep(2 * time.Second)
					observed = time.Now()
				case "foreign":
					session = "other"
				case "canceled":
					cancel()
				case "backward":
					observed = time.Now().Add(-time.Second)
				}
				runtime.mu.Lock() // Prove the heartbeat never queues behind native input.
				accepted, err := runtime.ObserveHeartbeatAhead(ctx, session, observed, 2*time.Second)
				runtime.mu.Unlock()
				if mode == "accepted" {
					if !accepted || err != nil || !runtime.Snapshot().Liveness.LastObserved.Equal(observed) {
						t.Fatal("valid heartbeat did not advance independently", accepted, err)
					}
				} else if accepted || runtime.LeaseWatch() != before {
					t.Fatal("invalid heartbeat renewed lease", accepted, err)
				}
			})
		})
	}
}

func TestHeartbeatAheadCannotKeepFailedInputOrRetiredLeaseAlive(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	grantInbound(t, runtime, time.Unix(100, 0))
	if err := runtime.Fail(context.Background(), context.Canceled); err == nil {
		t.Fatal("missing input failure")
	}
	before := runtime.LeaseWatch()
	accepted, err := runtime.ObserveHeartbeatAhead(context.Background(), "session-1", time.Unix(101, 0), 2*time.Second)
	if accepted || err != nil || runtime.LeaseWatch() != before || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("retired lease revived")
	}
}

func TestLeaseExpiryClaimCannotRaceRenewalOrReviveASealedLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime := newTestRuntime(t, &recordingInputPort{})
		grantInbound(t, runtime, time.Now())
		before := runtime.LeaseWatch()
		if runtime.ClaimLeaseExpiry(before, 2*time.Second) {
			t.Fatal("live lease sealed")
		}
		time.Sleep(time.Second)
		if ok, err := runtime.ObserveHeartbeatAhead(context.Background(), "session-1", time.Now(), 2*time.Second); !ok || err != nil {
			t.Fatal(ok, err)
		}
		current := runtime.LeaseWatch()
		time.Sleep(time.Second)
		if runtime.ClaimLeaseExpiry(before, 2*time.Second) {
			t.Fatal("stale watchdog expired renewed lease")
		}
		time.Sleep(time.Second)
		if !runtime.ClaimLeaseExpiry(current, 2*time.Second) {
			t.Fatal("silent peer escaped expiry")
		}
		if ok, _ := runtime.ObserveHeartbeatAhead(context.Background(), "session-1", time.Now(), 2*time.Second); ok {
			t.Fatal("sealed lease revived")
		}
		faulted, err := runtime.CheckLivenessNow(context.Background(), time.Now, 2*time.Second)
		if !faulted || err != nil || runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatal("sealed lease cleanup failed", faulted, err)
		}
	})
}

func TestSealedLeaseRejectsInputEvenWithAnEarlierObservationClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		port := &recordingInputPort{}
		runtime := newTestRuntime(t, port)
		start := time.Now()
		grantInbound(t, runtime, start)
		watch := runtime.LeaseWatch()
		time.Sleep(2 * time.Second)
		if !runtime.ClaimLeaseExpiry(watch, 2*time.Second) {
			t.Fatal("expiry not sealed")
		}
		if err := runtime.ObserveHeartbeat(context.Background(), "session-1", start.Add(time.Second)); err == nil {
			t.Fatal("ordinary heartbeat revived sealed lease")
		}
		err := runtime.HandleInputActionWithLiveness(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, func() time.Time { return start.Add(time.Second) }, 2*time.Second)
		if err == nil || len(port.moves) != 0 || runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatal("sealed lease admitted input", err)
		}
	})
}
