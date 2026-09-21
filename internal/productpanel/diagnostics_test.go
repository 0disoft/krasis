package productpanel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

type diagnosticClient struct {
	peers    []localapi.TrustedPeer
	result   localapi.DiagnosticTransferResult
	status   localapi.UpdateResult
	sendErr  error
	trustErr error
	sent     string
	request  localapi.UpdateRequest
}

func (c *diagnosticClient) Trust(context.Context) (localapi.Trust, error) {
	return localapi.Trust{Peers: c.peers}, c.trustErr
}
func (c *diagnosticClient) Update(_ context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	c.request = r
	return c.status, nil
}
func (c *diagnosticClient) SendDiagnostic(_ context.Context, peer string) (localapi.DiagnosticTransferResult, error) {
	c.sent = peer
	return c.result, c.sendErr
}

func TestDiagnosticsLocalSaveWithoutAgentAndSaveFailureBlocksDelivery(t *testing.T) {
	for _, fail := range []bool{false, true} {
		opened := false
		b := NewDiagnosticsBridge(context.Background(), func() (DiagnosticClient, error) { opened = true; return nil, errors.New("secret endpoint") }, func() error {
			if fail {
				return errors.New("secret path")
			}
			return nil
		}, func() (bool, bool, error) { return false, true, nil }, func() error { return nil })
		v := b.Export("")
		if opened || v.Saved == fail || !v.Received || (fail && v.Code != "save_failed") || (!fail && v.Code != "ok") {
			t.Fatalf("unexpected save: %+v", v)
		}
		if fail {
			v = b.Export(strings.Repeat("a", 64))
			if opened || v.Code != "save_failed" {
				t.Fatal("save failure attempted delivery")
			}
		}
	}
}

func TestDiagnosticsRechecksTrustAndPreservesLocalEvidence(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, peer          string
		revoked, trustError bool
		want                string
	}{
		{"valid", id, false, false, "queued"}, {"revoked", id, true, false, "stale_peer"}, {"different", strings.Repeat("b", 64), false, false, "stale_peer"}, {"unreadable", id, false, true, "unavailable"}, {"invalid", "../private", false, false, "invalid_peer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &diagnosticClient{peers: []localapi.TrustedPeer{{Fingerprint: id, Revoked: tc.revoked}}, result: localapi.DiagnosticTransferResult{Code: "queued", Path: "secret"}}
			if tc.trustError {
				c.trustErr = errors.New("token")
			}
			saves := 0
			b := NewDiagnosticsBridge(context.Background(), func() (DiagnosticClient, error) { return c, nil }, func() error { saves++; return nil }, func() (bool, bool, error) { return false, false, nil }, func() error { return nil })
			v := b.Export(tc.peer)
			if v.Code != tc.want || (c.sent != "") != (tc.want == "queued") {
				t.Fatalf("unexpected delivery: %+v %q", v, c.sent)
			}
			if tc.want == "invalid_peer" {
				if saves != 0 {
					t.Fatal("invalid request wrote a file")
				}
			} else if !v.Saved {
				t.Fatal("lost local evidence")
			}
			if tc.want == "queued" && v.Transfer != "QUEUED" {
				t.Fatal("queue is not completion")
			}
		})
	}
}

func TestDiagnosticsStatusRedactsPathsAndUnknownValues(t *testing.T) {
	for _, state := range []string{"", "QUEUED", "SENT", "EXPIRED", "UNSUPPORTED", "REJECTED", "secret-token"} {
		c := &diagnosticClient{status: localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Directory: "private", Status: peerupdate.Status{Diagnostic: state, ReceivedDiagnostic: "secret-path", Peer: "secret-key"}}}
		b := NewDiagnosticsBridge(context.Background(), func() (DiagnosticClient, error) { return c, nil }, func() error { return nil }, func() (bool, bool, error) { return true, true, nil }, func() error { return nil })
		v := b.Status()
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "private") || !v.Saved || !v.Received || c.request.Action != "STATUS" {
			t.Fatalf("invalid projection: %s", raw)
		}
		if state == "secret-token" && v.Transfer != "UNKNOWN" {
			t.Fatal("unknown transfer promoted")
		}
	}
}

func TestDiagnosticsAmbiguousSendAndFolderFailureAreNotSuccess(t *testing.T) {
	id := strings.Repeat("a", 64)
	c := &diagnosticClient{peers: []localapi.TrustedPeer{{Fingerprint: id}}, sendErr: errors.New("bearer token")}
	b := NewDiagnosticsBridge(context.Background(), func() (DiagnosticClient, error) { return c, nil }, func() error { return nil }, func() (bool, bool, error) { return true, false, nil }, func() error { return errors.New("private path") })
	if v := b.Export(id); v.Code != "outcome_unknown" || !v.Saved || v.Transfer != "UNKNOWN" {
		t.Fatalf("ambiguous send: %+v", v)
	}
	if v := b.OpenFolder(); v.Code != "open_failed" {
		t.Fatalf("folder failure: %+v", v)
	}
}

func TestDiagnosticsBusyAndShutdownDoNotQueueNewEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	b := NewDiagnosticsBridge(ctx, func() (DiagnosticClient, error) { return nil, nil }, func() error { close(entered); <-release; return nil }, func() (bool, bool, error) { return false, false, nil }, func() error { t.Error("folder unexpectedly opened"); return nil })
	done := make(chan DiagnosticsSnapshot, 1)
	go func() { done <- b.Export("") }()
	<-entered
	if b.OpenFolder().Code != "busy" || b.Export("").Code != "busy" {
		t.Error("overlapping mutation was queued")
	}
	close(release)
	<-done
	cancel()
	if b.Export("").Code != "cancelled" {
		t.Fatal("shutdown accepted new write")
	}
	var absent *DiagnosticsBridge
	if absent.Status().Code != "unavailable" {
		t.Fatal("smoke bridge is not disabled")
	}
}
