package productpanel

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

func waitDiagnosticWorker(t *testing.T, b *DiagnosticsBridge) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		done := !b.working
		b.mu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("diagnostic worker did not finish")
}

func TestDiagnosticSaveReturnsWhileLocalDiskIsBlocked(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{})
	var saves atomic.Int32
	b := collectionBridge(nil, func() error { saves.Add(1); close(entered); <-gate; return nil })
	if v := b.BeginCollect(); v.Code != "saving" || v.Saved {
		t.Fatal(v)
	}
	<-entered
	defer func() { close(gate); waitDiagnosticWorker(t, b) }()
	for i := 0; i < 3; i++ {
		if v := b.Status(); v.Code != "saving" || v.Saved {
			t.Fatal(v)
		}
		if v := b.BeginCollect(); v.Code != "saving" {
			t.Fatal(v)
		}
	}
	if saves.Load() != 1 {
		t.Fatal("overlapping save worker")
	}
}

func TestDiagnosticPollDoesNotHoldBridgeOrRepeatStart(t *testing.T) {
	c := collectionFixture()
	gate, entered := make(chan struct{}), make(chan struct{})
	var starts atomic.Int32
	c.reply = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Action == "START" {
			starts.Add(1)
			return localapi.DiagnosticCollectionResult{Code: "queued", ID: strings.Repeat("c", 32), LocalPath: "private"}, nil
		}
		close(entered)
		<-gate
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: r.ID, State: "RECEIVED", LocalPath: "A", PeerPath: "B"}, nil
	}
	b := collectionBridge(c, func() error { t.Error("duplicated local export"); return nil })
	b.BeginCollect()
	waitDiagnosticWorker(t, b)
	if v := b.Status(); v.Code != "pending" {
		t.Fatal(v)
	}
	<-entered
	for i := 0; i < 3; i++ {
		if v := b.Status(); v.Code != "pending" {
			t.Fatal(v)
		}
		b.BeginCollect()
	}
	close(gate)
	waitDiagnosticWorker(t, b)
	if v := b.Status(); v.Code != "collected" || !v.Received {
		t.Fatal(v)
	}
	if starts.Load() != 1 {
		t.Fatal("START retried during poll")
	}
}
