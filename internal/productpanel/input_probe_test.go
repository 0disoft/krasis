package productpanel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestStartupHealthProbeDeadlineRetainsSingleReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, finish := make(chan struct{}), make(chan struct{})
		var probes atomic.Int32
		b := &inputRefresh{inspect: func() string {
			probes.Add(1)
			close(entered)
			<-finish
			return "UPDATE_REQUIRED"
		}}
		b.setState("READY") // The previous observation cannot authorize maintenance.
		s := Startup{InputProbe: b.probe, Open: func() (RuntimeClient, error) {
			t.Error("expired preflight continued to agent inspection")
			return nil, errors.New("unexpected agent inspection")
		}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result := make(chan error, 1)
		elapsed := make(chan time.Duration, 1)
		go func() {
			started := time.Now()
			needed, err := s.LaunchMaintenanceNeeded(ctx)
			if needed {
				t.Error("expired preflight authorized maintenance")
			}
			elapsed <- time.Since(started)
			result <- err
		}()
		<-entered
		time.Sleep(2 * time.Second)
		select {
		case err := <-result:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("preflight lost deadline", err)
			}
		default:
			close(finish)
			<-result
			t.Fatal("preflight exceeded deadline while native health inspection blocked")
		}
		if waited := <-elapsed; waited != time.Second {
			t.Fatal("preflight did not return at its original deadline", waited)
		}
		reader := b.reader.Load()
		if reader == nil {
			t.Fatal("unfinished inspection lost its owner")
		}
		for range 3 {
			b.probe(ctx)
			b.status()
			b.recheck()
		}
		if probes.Load() != 1 || !errors.Is(b.explicit(context.Background()), ErrStartupBusy) {
			t.Fatal("expired probe duplicated work or released the installation gate", probes.Load())
		}
		close(finish)
		<-reader.done
		if b.status() != "UPDATE_REQUIRED" || probes.Load() != 1 {
			t.Fatal("owned inspection did not publish its actual outcome", b.status(), probes.Load())
		}
	})
}

func TestStartupHealthProbeSharesExistingRestorationReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, finish := make(chan struct{}), make(chan struct{})
		var probes atomic.Int32
		b := &inputRefresh{inspect: func() string {
			if probes.Add(1) == 1 {
				close(entered)
				<-finish
			}
			return "READY"
		}}
		b.status()
		<-entered
		probed, restored := make(chan string, 1), make(chan error, 1)
		go func() { probed <- b.probe(context.Background()) }()
		go func() { restored <- b.restoreCheck(context.Background()) }()
		synctest.Wait()
		close(finish)
		if state, err := <-probed, <-restored; state != "READY" || err != nil || probes.Load() != 1 {
			t.Fatal("waiters repeated or lost their shared observation", state, err, probes.Load())
		}
	})
}
