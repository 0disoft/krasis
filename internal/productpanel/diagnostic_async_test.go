package productpanel

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
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

func TestDiagnosticRestartResumesStatusWithoutStartingAnotherRequest(t *testing.T) {
	c := collectionFixture()
	id, peer := strings.Repeat("c", 32), strings.Repeat("b", 64)
	var polls atomic.Int32
	c.reply = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Action != "STATUS" || r.ID != id || r.Peer != peer {
			t.Error("restored collection issued START or changed identity")
		}
		polls.Add(1)
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "RECEIVED", LocalPath: "A", PeerPath: "B"}, nil
	}
	b := collectionBridge(c, func() error { t.Error("restored poll exported again"); return nil })
	var saved productlog.CollectionReceipt
	b.ConfigureCollectionStorage(func() (*productlog.CollectionReceipt, error) {
		return &productlog.CollectionReceipt{Schema: "krasis.diagnostic-collection/v1", ID: id, Peer: peer, Expires: time.Now().Add(time.Minute), State: "PENDING", Code: "pending", LocalSaved: true}, nil
	}, func(r productlog.CollectionReceipt) error { saved = r; return nil })
	if v := b.BeginCollect(); v.Code != "pending" {
		t.Fatal(v)
	}
	waitDiagnosticWorker(t, b)
	if v := b.Status(); v.Code != "collected" || !v.Received {
		t.Fatal(v)
	}
	if polls.Load() != 1 || saved.ID != id || saved.State != "RECEIVED" {
		t.Fatal(saved, polls.Load())
	}
}

func TestDiagnosticReceiptFailurePreventsStartButKeepsLocalFallback(t *testing.T) {
	c := collectionFixture()
	c.reply = func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		t.Error("START without persisted intent")
		return localapi.DiagnosticCollectionResult{}, nil
	}
	b := collectionBridge(c, func() error { return nil })
	b.ConfigureCollectionStorage(func() (*productlog.CollectionReceipt, error) { return nil, nil }, func(productlog.CollectionReceipt) error { return errors.New("private") })
	b.BeginCollect()
	waitDiagnosticWorker(t, b)
	if v := b.Status(); v.Code != "state_save_failed" || !v.Saved || v.Received {
		t.Fatal(v)
	}
}
