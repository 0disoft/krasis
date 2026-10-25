package peerupdate

import (
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestAppOpenReadinessDoesNotBlockStatusOrQueue(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	entered, release := make(chan struct{}), make(chan struct{})
	m := &Manager{OpenUI: func(string) string { return "launch_requested" }, UIReady: func(time.Time) bool {
		close(entered)
		<-release
		return true
	}}
	m.appOpen.received = map[string]appOpenReceipt{peer: {id: id, code: "launch_requested", at: time.Now()}}
	allowed := func() bool { return true }
	checking := make(chan string, 1)
	go func() { checking <- m.acceptAppOpen(peer, id, allowed) }()
	<-entered
	observed := make(chan string, 1)
	go func() {
		queued, err := m.QueueAppOpen(peer)
		if err != nil {
			observed <- "queue failed"
			return
		}
		observed <- m.AppOpenStatus(peer, queued)
	}()
	returned := false
	select {
	case status := <-observed:
		returned = true
		if status != "pending" {
			t.Error("unexpected queued status", status)
		}
	case <-time.After(time.Second):
		t.Error("readiness I/O blocked an unrelated queue/status operation")
	}
	close(release)
	if code := <-checking; code != "window_ready" {
		t.Error("readiness result lost", code)
	}
	if !returned {
		<-observed
	} // Always join the old-code regression fixture.
}

func TestAppOpenReadinessHasOneIOOwner(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	other := strings.Repeat("c", 64)
	entered, release := make(chan struct{}), make(chan struct{})
	var checks atomic.Int32
	m := &Manager{OpenUI: func(string) string { return "launch_requested" }, UIReady: func(time.Time) bool {
		if checks.Add(1) == 1 {
			close(entered)
		}
		<-release
		return true
	}}
	m.appOpen.received = map[string]appOpenReceipt{
		peer:  {id: id, code: "launch_requested", at: time.Now()},
		other: {id: id, code: "launch_requested", at: time.Now()},
	}
	checking := make(chan string, 1)
	go func() { checking <- m.acceptAppOpen(peer, id, func() bool { return true }) }()
	<-entered
	returned := make(chan string, 1)
	go func() { returned <- m.acceptAppOpen(other, id, func() bool { return true }) }()
	completed := false
	select {
	case code := <-returned:
		completed = true
		if code != "launch_requested" {
			t.Error("contending poll changed its receipt", code)
		}
	case <-time.After(time.Second):
		t.Error("contending readiness poll queued behind image I/O")
	}
	if n := checks.Load(); n != 1 {
		t.Error("multiple readiness callbacks in flight", n)
	}
	close(release)
	<-checking
	if !completed {
		<-returned
	}
	if code := m.acceptAppOpen(other, id, func() bool { return true }); code != "window_ready" || checks.Load() != 2 {
		t.Fatal("later poll did not obtain its own fresh verification", code, checks.Load())
	}
}

func TestAppOpenReadinessRevalidatesAfterIO(t *testing.T) {
	for _, change := range []string{"replacement", "expired", "revoked", "terminal"} {
		t.Run(change, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
				entered, release := make(chan struct{}), make(chan struct{})
				var allowed atomic.Bool
				allowed.Store(true)
				m := &Manager{OpenUI: func(string) string { return "launch_requested" }, UIReady: func(time.Time) bool {
					close(entered)
					<-release
					return true
				}}
				m.appOpen.received = map[string]appOpenReceipt{peer: {id: id, code: "launch_requested", at: time.Now()}}
				done := make(chan string, 1)
				go func() { done <- m.acceptAppOpen(peer, id, allowed.Load) }()
				<-entered
				want := "rejected"
				switch change {
				case "replacement", "terminal":
					m.appOpen.mu.Lock()
					r := m.appOpen.received[peer]
					if change == "replacement" {
						r.id = strings.Repeat("c", 32)
					} else {
						r.code, want = "launch_failed", "launch_failed"
					}
					m.appOpen.received[peer] = r
					m.appOpen.mu.Unlock()
				case "expired":
					time.Sleep(2 * time.Minute)
				case "revoked":
					allowed.Store(false)
				}
				close(release)
				if code := <-done; code != want {
					t.Fatal("obsolete readiness published", code, want)
				}
				if r := m.appOpen.received[peer]; r.code == "window_ready" {
					t.Fatal("changed receipt overwritten by old I/O")
				}
			})
		})
	}
}
