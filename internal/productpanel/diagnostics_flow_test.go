package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type diagnosticFlowClient struct {
	*diagnosticClient
	calls      []string
	updateErr  error
	afterTrust func()
}

func (c *diagnosticFlowClient) Trust(ctx context.Context) (localapi.Trust, error) {
	c.calls = append(c.calls, "TRUST")
	if c.afterTrust != nil {
		c.afterTrust()
	}
	return c.diagnosticClient.Trust(ctx)
}

func (c *diagnosticFlowClient) Update(ctx context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	c.calls = append(c.calls, r.Action)
	if c.updateErr != nil {
		return localapi.UpdateResult{}, c.updateErr
	}
	return c.diagnosticClient.Update(ctx, r)
}

func (c *diagnosticFlowClient) SendDiagnostic(ctx context.Context, peer string) (localapi.DiagnosticTransferResult, error) {
	c.calls = append(c.calls, "SEND")
	return c.diagnosticClient.SendDiagnostic(ctx, peer)
}

func TestDiagnosticsFlowPreservesEffectsAndEvidence(t *testing.T) {
	peer := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, action, peer, failure, code, calls string
		saved                                    bool
	}{
		{"local export", "export", "", "", "ok", "FILES EXPORT", true},
		{"remote export", "export", peer, "", "queued", "FILES EXPORT OPEN TRUST SEND", true},
		{"short fingerprint", "export", peer[:63], "", "invalid_peer", "FILES", false},
		{"non-hex fingerprint", "export", strings.Repeat("g", 64), "", "invalid_peer", "FILES", false},
		{"file status failure", "status", "", "files", "unavailable", "FILES", false},
		{"file listing failure still saves", "export", peer, "files", "queued", "FILES EXPORT OPEN TRUST SEND", true},
		{"save failure", "export", peer, "export", "save_failed", "FILES EXPORT", false},
		{"offline agent", "export", peer, "open", "unavailable", "FILES EXPORT OPEN", true},
		{"revoked peer", "export", peer, "revoked", "stale_peer", "FILES EXPORT OPEN TRUST", true},
		{"trust lookup failure", "export", peer, "trust", "unavailable", "FILES EXPORT OPEN TRUST", true},
		{"lost send reply", "export", peer, "send", "outcome_unknown", "FILES EXPORT OPEN TRUST SEND", true},
		{"status does not send", "status", "", "", "ok", "FILES OPEN STATUS", false},
		{"status lookup failure", "status", "", "status", "unavailable", "FILES OPEN STATUS", false},
		{"folder open", "open", "", "", "ok", "FILES FOLDER", false},
		{"folder failure", "open", "", "folder", "open_failed", "FILES FOLDER", false},
		{"cancel after file listing", "export", peer, "cancel-files", "cancelled", "FILES EXPORT", true},
		{"cancel after remote save", "export", peer, "cancel-export", "cancelled", "FILES EXPORT", true},
		{"local save survives cancellation", "export", "", "cancel-export", "ok", "FILES EXPORT", true},
		{"cancel after trust", "export", peer, "cancel-trust", "cancelled", "FILES EXPORT OPEN TRUST", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &diagnosticFlowClient{diagnosticClient: &diagnosticClient{
				peers:  []localapi.TrustedPeer{{Fingerprint: peer, Revoked: tc.failure == "revoked"}},
				result: localapi.DiagnosticTransferResult{Code: "queued"}, status: localapi.UpdateResult{Code: "ok"},
			}}
			if tc.failure == "cancel-trust" {
				client.afterTrust = cancel
			}
			fail := func(stage string) error {
				if tc.failure == "cancel-"+stage {
					cancel()
				}
				if tc.failure == stage {
					return errors.New("private failure")
				}
				return nil
			}
			switch tc.failure {
			case "trust":
				client.trustErr = errors.New("trust failure")
			case "send":
				client.sendErr = errors.New("send failure")
			case "status":
				client.updateErr = errors.New("status failure")
			}
			b := NewDiagnosticsBridge(ctx, func() (DiagnosticClient, error) {
				client.calls = append(client.calls, "OPEN")
				return client, fail("open")
			}, func() error {
				client.calls = append(client.calls, "EXPORT")
				return fail("export")
			}, func() (bool, bool, error) {
				client.calls = append(client.calls, "FILES")
				return false, true, fail("files")
			}, func() error {
				client.calls = append(client.calls, "FOLDER")
				return fail("folder")
			})
			got := b.execute(tc.action, tc.peer)
			if got.Code != tc.code || got.Saved != tc.saved || !got.Received || strings.Join(client.calls, " ") != tc.calls {
				t.Fatalf("snapshot=%+v calls=%v", got, client.calls)
			}
		})
	}
}

func TestDiagnosticExportPeerBoundaries(t *testing.T) {
	for _, tc := range []struct {
		peer  string
		valid bool
	}{
		{"", true},
		{strings.Repeat("a", 63), false},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{strings.Repeat("A", 64), true},
		{strings.Repeat("g", 64), false},
	} {
		if got := validDiagnosticExportPeer(tc.peer); got != tc.valid {
			t.Fatalf("peer=%q valid=%t want=%t", tc.peer, got, tc.valid)
		}
	}
}

func TestDiagnosticProjectionsPreserveInputAndUncertainOutcome(t *testing.T) {
	collection := &DiagnosticCollectionSnapshot{State: "PENDING", LocalSaved: true}
	input := DiagnosticsSnapshot{Schema: DiagnosticsSchema, Code: "unavailable", Saved: true, Received: true, Transfer: "UNKNOWN", Collection: collection}
	before, collectionBefore := input, *collection
	status := projectDiagnosticStatus(input, localapi.UpdateResult{Code: "ok"})
	result := localapi.DiagnosticTransferResult{Code: "queued", Path: "private/path"}
	sent := projectDiagnosticSend(input, result, nil)
	lost := projectDiagnosticSend(input, result, errors.New("private error"))
	if input != before || *collection != collectionBefore || result.Path != "private/path" {
		t.Fatal("projection mutated caller-owned evidence")
	}
	if status.Code != "ok" || status.Transfer != "NONE" || sent.Code != "queued" || sent.Transfer != "QUEUED" {
		t.Fatalf("incorrect projection: status=%+v sent=%+v", status, sent)
	}
	if lost.Code != "outcome_unknown" || lost.Transfer != "UNKNOWN" || !lost.Saved || !lost.Received {
		t.Fatalf("lost reply became success or discarded local evidence: %+v", lost)
	}
}
