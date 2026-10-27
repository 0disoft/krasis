package inputworker

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestNativeExecutionUsesOneFreshDesktopAgentCheck(t *testing.T) {
	for _, denied := range []string{"", "service", "agent", "parent"} {
		t.Run(denied, func(t *testing.T) {
			serviceChecks, agentChecks, applied := 0, 0, 0
			execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
				agentChecks++
				if serviceChecks != 1 {
					t.Fatal("desktop checked before service RPC")
				}
				if denied == "agent" {
					return ErrAuthority
				}
				return work()
			}, func() error {
				serviceChecks++
				if denied == "service" {
					return ErrAuthority
				}
				return nil
			}, func() bool { return denied != "parent" }, time.Now().Add(time.Second), false)
			err := execute(context.Background(), func() error { applied++; return nil })
			if (err == nil) != (denied == "") || (applied == 1) != (denied == "") || serviceChecks != 1 || agentChecks > 1 || denied != "service" && agentChecks != 1 {
				t.Fatal("duplicate proof or denied native execution", err, serviceChecks, agentChecks, applied)
			}
		})
	}
}

func TestNativeExecutionRechecksExpiryAndCancellationAfterSlowProof(t *testing.T) {
	for _, mode := range []string{"expiry", "cancel", "release"} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			applied := 0
			execute := nativeInputExecutor(func(_ context.Context, work func() error) error { return work() }, func() error {
				time.Sleep(60 * time.Millisecond)
				if mode == "cancel" {
					cancel()
				}
				return nil
			}, func() bool { return true }, time.Now().Add(50*time.Millisecond), mode == "release")
			err := execute(ctx, func() error { applied++; return nil })
			if (err == nil) != (mode == "release") || (applied == 1) != (mode == "release") {
				t.Fatal("proof delay bypassed expiry/cancellation or blocked recovery", mode, err, applied)
			}
		})
	}
}
