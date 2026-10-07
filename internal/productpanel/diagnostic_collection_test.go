package productpanel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type collectionClient struct {
	*diagnosticClient
	active    localapi.Status
	statusErr error
	requests  []localapi.DiagnosticCollectionRequest
	reply     func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error)
}

func (c *collectionClient) Status(context.Context) (localapi.Status, error) {
	return c.active, c.statusErr
}
func (c *collectionClient) CollectDiagnostic(_ context.Context, request localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
	c.requests = append(c.requests, request)
	return c.reply(request)
}

func collectionFixture() *collectionClient {
	return &collectionClient{diagnosticClient: &diagnosticClient{peers: []localapi.TrustedPeer{
		{Alias: "Other", Fingerprint: strings.Repeat("a", 64)},
		{Alias: "B", Fingerprint: strings.Repeat("b", 64)},
	}}, active: localapi.Status{State: "CONNECTED", PeerAlias: "B"}}
}

func collectionBridge(client DiagnosticClient, export func() error) *DiagnosticsBridge {
	return NewDiagnosticsBridge(context.Background(), func() (DiagnosticClient, error) { return client, nil }, export,
		func() (bool, bool, error) { return true, true, nil }, func() error { return nil })
}

func TestDiagnosticCollectionPinsActivePeerAndNeverConfusesOlderFiles(t *testing.T) {
	c := collectionFixture()
	id := strings.Repeat("c", 32)
	ready := false
	c.reply = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
		if r.Peer != strings.Repeat("b", 64) {
			t.Fatal("collection chose a registered but inactive peer")
		}
		if r.Action == "START" {
			return localapi.DiagnosticCollectionResult{Code: "queued", ID: id, LocalPath: "private-A"}, nil
		}
		if r.Action != "STATUS" || r.ID != id {
			t.Fatal("poll lost its original peer/request")
		}
		if !ready {
			return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "PENDING"}, nil
		}
		return localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "RECEIVED", LocalPath: "private-A", PeerPath: "private-B"}, nil
	}
	saves := 0
	b := collectionBridge(c, func() error { saves++; return nil })
	first := b.Collect()
	if first.Code != "pending" || !first.Saved || first.Received || first.Collection.State != "PENDING" {
		t.Fatal("old peer file was mistaken for fresh evidence", first)
	}
	if second := b.Collect(); second.Code != "pending" || saves != 0 {
		t.Fatal("repeat click started a second collection", second, saves)
	}
	ready = true
	done := b.Status()
	if done.Code != "collected" || !done.Saved || !done.Received || done.Collection.State != "RECEIVED" {
		t.Fatal("fresh pair did not complete", done)
	}
	if first.Collection.PeerSaved {
		t.Fatal("later status mutated an earlier snapshot")
	}
	starts := 0
	for _, r := range c.requests {
		if r.Action == "START" {
			starts++
		}
	}
	if starts != 1 {
		t.Fatal("status polling retried START", starts)
	}
	raw, _ := json.Marshal(done)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), id) || strings.Contains(string(raw), strings.Repeat("b", 64)) {
		t.Fatal("request identity or private path entered UI", string(raw))
	}
}

func TestDiagnosticCollectionKeepsLocalFallbackAndRejectsUnsafeTargets(t *testing.T) {
	for _, name := range []string{"absent", "offline", "faulted", "status_error", "trust_error", "revoked", "duplicate_alias", "invalid_identity", "unsupported", "storage_limit", "lost_start", "malformed_start", "save_failure"} {
		t.Run(name, func(t *testing.T) {
			c := collectionFixture()
			want := "partial"
			wantStarts := 0
			var exportErr error
			var client DiagnosticClient = c
			c.reply = func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
				t.Fatal("unexpected remote start")
				return localapi.DiagnosticCollectionResult{}, nil
			}
			switch name {
			case "absent":
				client = nil
			case "offline":
				c.active.State, want = "LOCAL", "local_only"
			case "faulted":
				c.active.State = "FAULTED"
			case "status_error":
				c.statusErr = errors.New("private")
			case "trust_error":
				c.trustErr = errors.New("private")
			case "revoked":
				c.peers[1].Revoked, want = true, "stale_peer"
			case "duplicate_alias":
				c.peers = append(c.peers, localapi.TrustedPeer{Alias: "B", Fingerprint: strings.Repeat("d", 64)})
				want = "stale_peer"
			case "invalid_identity":
				c.peers[1].Fingerprint, want = "private", "stale_peer"
			case "unsupported":
				client, want = c.diagnosticClient, "unsupported"
			case "storage_limit":
				want, wantStarts = "storage_limit", 1
				c.reply = func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
					return localapi.DiagnosticCollectionResult{Code: "storage_limit"}, nil
				}
			case "lost_start":
				want, wantStarts = "outcome_unknown", 1
				c.reply = func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
					return localapi.DiagnosticCollectionResult{}, errors.New("private token")
				}
			case "malformed_start":
				want, wantStarts = "outcome_unknown", 1
				c.reply = func(localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
					return localapi.DiagnosticCollectionResult{Code: "queued", ID: strings.Repeat("c", 32)}, nil
				}
			case "save_failure":
				client = nil
				exportErr, want = errors.New("private path"), "save_failed"
			}
			b := collectionBridge(client, func() error { return exportErr })
			v := b.Collect()
			if v.Code != want || v.Saved != (exportErr == nil) || v.Received || v.Collection.PeerSaved || len(c.requests) != wantStarts {
				t.Fatalf("lost fallback or unsafe collection: %+v requests=%v", v, c.requests)
			}
			b.Status()
			b.Status()
			if len(c.requests) != wantStarts {
				t.Fatal("failed START was automatically retried")
			}
		})
	}
}

func TestDiagnosticCollectionOnlyCompletesForExactReceivedPair(t *testing.T) {
	for _, name := range []string{"pending", "received", "late_received", "missing_local", "missing_peer", "wrong_id", "unknown", "expired", "rejected", "poll_error", "local_timeout"} {
		t.Run(name, func(t *testing.T) {
			c := collectionFixture()
			id := strings.Repeat("c", 32)
			c.reply = func(r localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
				if r.Action == "START" {
					return localapi.DiagnosticCollectionResult{Code: "queued", ID: id, LocalPath: "A"}, nil
				}
				v := localapi.DiagnosticCollectionResult{Code: "ok", ID: id, State: "RECEIVED", LocalPath: "A", PeerPath: "B"}
				switch name {
				case "pending", "local_timeout":
					v.State = "PENDING"
				case "missing_local":
					v.LocalPath = ""
				case "missing_peer":
					v.PeerPath = ""
				case "wrong_id":
					v.ID = strings.Repeat("d", 32)
				case "unknown":
					v.State = "UNKNOWN"
				case "expired":
					v.State = "EXPIRED"
				case "rejected":
					v.Code = "rejected"
				case "poll_error":
					return v, errors.New("private")
				}
				return v, nil
			}
			b := collectionBridge(c, func() error { return nil })
			b.Collect()
			if name == "local_timeout" || name == "late_received" {
				b.collection.expires = time.Now().Add(-time.Second)
			}
			v := b.Status()
			if !v.Saved || v.Received != (name == "received" || name == "late_received") {
				t.Fatal("incomplete pair advertised as received", v)
			}
			if (name == "received" || name == "late_received") && v.Code != "collected" {
				t.Fatal(v)
			}
			if name == "poll_error" && (v.Code != "outcome_unknown" || v.Collection.State != "PENDING") {
				t.Fatal("lost poll abandoned or completed request", v)
			}
			if name == "local_timeout" && (v.Code != "partial" || v.Collection.State != "EXPIRED") {
				t.Fatal("unanswered collection did not expire", v)
			}
			starts := 0
			for _, r := range c.requests {
				if r.Action == "START" {
					starts++
				}
			}
			if starts != 1 {
				t.Fatal("poll retried collection")
			}
		})
	}
}
