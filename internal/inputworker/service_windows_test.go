//go:build windows

package inputworker

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestServiceRejectsZeroInstallationWithoutSCMOrInput(t *testing.T) {
	var installation Installation
	if RunService(&installation) != ErrAuthority || RunInstalledWorker(context.Background(), &installation) != ErrAuthority || RunInstalledLoginConnection(context.Background(), &installation) != ErrAuthority {
		t.Fatal("uninstalled entry point accepted")
	}
}

func TestServiceStopReportsPendingUntilWorkerRecoveryFinishes(t *testing.T) {
	changes := make(chan svc.ChangeRequest)
	updates := make(chan svc.Status, 8)
	cancelSeen := make(chan struct{})
	recovery := make(chan struct{})
	handler := &serviceHandler{run: func(ctx context.Context) error { <-ctx.Done(); close(cancelSeen); <-recovery; return nil }}
	type result struct {
		specific bool
		code     uint32
	}
	done := make(chan result, 1)
	go func() {
		specific, code := handler.Execute([]string{ServiceName}, changes, updates)
		done <- result{specific, code}
	}()
	if (<-updates).State != svc.StartPending || (<-updates).State != svc.Running {
		t.Fatal("invalid service startup states")
	}
	changes <- svc.ChangeRequest{Cmd: svc.Stop}
	if (<-updates).State != svc.StopPending {
		t.Fatal("stop pending missing")
	}
	<-cancelSeen
	select {
	case <-done:
		t.Fatal("SCM completion preceded recovery")
	default:
	}
	changes <- svc.ChangeRequest{Cmd: svc.Interrogate}
	if (<-updates).State != svc.StopPending {
		t.Fatal("interrogate lost stop state")
	}
	close(recovery)
	select {
	case result := <-done:
		if result.specific || result.code != 0 {
			t.Fatal("clean service stop failed")
		}
	case <-time.After(time.Second):
		t.Fatal("service stop stuck")
	}
}

func TestServiceReportsFailedRecoveryAsServiceFailure(t *testing.T) {
	updates := make(chan svc.Status, 8)
	handler := &serviceHandler{run: func(context.Context) error { return ErrRecovery }}
	specific, code := handler.Execute([]string{ServiceName}, nil, updates)
	if !specific || code == 0 {
		t.Fatal("failed recovery hidden from SCM")
	}
}
