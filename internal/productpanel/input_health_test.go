package productpanel

import (
	"context"
	"testing"
	"time"
)

func TestInputHealthRechecksWithoutBlockingPollOrStartingInstaller(t *testing.T) {
	state, probes, installs := "READY", 0, 0
	entered, finish := make(chan struct{}), make(chan struct{})
	b := &inputRefresh{inspect: func() string {
		probes++
		if probes == 2 {
			close(entered)
			<-finish
		}
		return state
	}, run: func(context.Context) error { installs++; return nil }}
	if b.status() != "READY" {
		t.Fatal("initial ready installation not observed")
	}
	for range 3 {
		b.status()
	}
	if probes != 1 {
		t.Fatal("each poll rehashed installation", probes)
	}
	state = "UNKNOWN" // The protected service stopped after the initial check.
	b.mu.Lock()
	b.checkedAt = time.Now().Add(-31 * time.Second)
	b.mu.Unlock()
	if b.status() != "READY" {
		t.Fatal("background query blocked or discarded prior observation")
	}
	<-entered
	if b.status() != "READY" {
		t.Fatal("poll waited for slow installation check")
	}
	close(finish)
	b.mu.Lock() // Join the read-only observation before inspecting the result.
	b.mu.Unlock()
	if b.status() != "UNKNOWN" || probes != 2 || installs != 0 {
		t.Fatal("service failure was hidden or triggered installation", b.status(), probes, installs)
	}
	state = "READY"
	b.mu.Lock()
	b.checkedAt = time.Now().Add(-31 * time.Second)
	b.mu.Unlock()
	b.status()
	b.mu.Lock()
	b.mu.Unlock()
	if b.status() != "READY" || probes != 3 || installs != 0 {
		t.Fatal("restored service was not observed", b.status(), probes, installs)
	}
}

func TestHealthPollPreservesTerminalConsentResult(t *testing.T) {
	for _, state := range []string{"CANCELLED", "FAILED", "UPDATING"} {
		b := &inputRefresh{inspect: func() string { t.Fatal("terminal outcome was silently replaced"); return "READY" }}
		b.mu.Lock()
		b.setState(state)
		b.checkedAt = time.Now().Add(-time.Minute)
		b.mu.Unlock()
		if b.status() != state {
			t.Fatal("terminal outcome lost", b.status())
		}
	}
}

func TestManualInputHealthRecheckBypassesCacheAndCoalescesReaders(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	b := &inputRefresh{inspect: func() string { close(entered); <-finish; return "UNKNOWN" }, run: func(context.Context) error { t.Fatal("manual check installed"); return nil }}
	b.mu.Lock()
	b.setState("READY")
	b.mu.Unlock()
	if b.status() != "READY" {
		t.Fatal("discarded cached health")
	}
	if b.recheck() != "READY" {
		t.Fatal("manual check waited for reader")
	}
	<-entered
	for range 3 {
		if b.recheck() != "READY" {
			t.Fatal("duplicate reader or discarded health")
		}
	}
	close(finish)
	b.mu.Lock()
	b.mu.Unlock()
	if b.status() != "UNKNOWN" {
		t.Fatal("manual check did not publish new health")
	}
}
