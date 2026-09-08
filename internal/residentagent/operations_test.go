package residentagent

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestConcurrentDisconnectWaitersAllObserveSessionCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		operations := testOperations(t, func(ctx context.Context, _ string) error {
			<-ctx.Done()
			<-release
			return context.Cause(ctx)
		})
		if result, err := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil || result.Status != "COMPLETED" {
			t.Fatalf("connect = %+v, %v", result, err)
		}
		results := make(chan localapi.OperationResult, 2)
		for range 2 {
			go func() {
				result, err := operations.Disconnect(context.Background(), localapi.OperationRequest{})
				if err != nil {
					t.Errorf("disconnect: %v", err)
				}
				results <- result
			}()
		}
		synctest.Wait()
		close(release)
		for range 2 {
			if result := <-results; result.Status != "COMPLETED" || result.Code != "disconnected" {
				t.Errorf("disconnect result = %+v", result)
			}
		}
		status, err := operations.Status(context.Background())
		if err != nil || status.State != "LOCAL" || status.PeerAlias != "" {
			t.Fatalf("status after completed disconnect = %+v, %v", status, err)
		}
	})
}

func TestConnectSessionOutlivesRequestAndDisconnectStopsIt(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	runner := func(ctx context.Context, alias string) error {
		if alias != "peer" {
			t.Errorf("alias = %q", alias)
		}
		close(started)
		<-ctx.Done()
		close(stopped)
		return context.Cause(ctx)
	}
	operations := testOperations(t, runner)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	result, err := operations.Connect(requestCtx, localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatalf("Connect() result = %+v error = %v", result, err)
	}
	<-started
	cancelRequest()
	select {
	case <-stopped:
		t.Fatal("request cancellation stopped the resident session")
	case <-time.After(20 * time.Millisecond):
	}
	result, err = operations.Disconnect(context.Background(), localapi.OperationRequest{})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatalf("Disconnect() result = %+v error = %v", result, err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not stop the resident session")
	}
}

func TestSafeStopCancelsAgentRoot(t *testing.T) {
	operations := testOperations(t, nil)
	result, err := operations.SafeStop(context.Background())
	if err != nil || result.Code != "safe_stop_requested" {
		t.Fatalf("SafeStop() result = %+v error = %v", result, err)
	}
	select {
	case <-operations.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("safe stop did not cancel agent root")
	}
}

func TestStatusAndDiagnosticsAreRedactedAggregates(t *testing.T) {
	operations := testOperations(t, nil)
	status, err := operations.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.DeviceFingerprint != strings.Repeat("a", 64) || status.IdentityGeneration != 3 || status.State != "LOCAL" {
		t.Fatalf("status = %+v", status)
	}
	result, err := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Code != "not_configured" {
		t.Fatalf("Connect() result = %+v error = %v", result, err)
	}
	diagnostics, err := operations.Diagnostics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Counters["not_configured"] != 1 || diagnostics.State != "not_configured" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestTrustTopologyAndRevocationStayAgentOwned(t *testing.T) {
	store := &stubTrustStore{document: truststore.Document{
		SchemaVersion: truststore.SchemaVersion,
		Revision:      4,
		UpdatedAt:     time.Now().UTC(),
		Peers: []truststore.PeerRecord{{
			Fingerprint: strings.Repeat("b", 64), Role: transport.AuthenticatedPeerConnector,
			Alias: "peer", AllowInboundControl: true, CreatedAt: time.Now().UTC(),
		}},
	}}
	operations := testOperationsWithTrust(t, store)
	trust, err := operations.Trust(context.Background())
	if err != nil || trust.Revision != 4 || len(trust.Peers) != 1 || trust.Peers[0].Fingerprint == "" {
		t.Fatalf("Trust() = %+v, %v", trust, err)
	}
	topology, err := operations.Topology(context.Background())
	if err != nil || topology.Layout != "UNCONFIGURED" || len(topology.Peers) != 1 {
		t.Fatalf("Topology() = %+v, %v", topology, err)
	}
	result, err := operations.RevokeTrust(context.Background(), localapi.OperationRequest{Fingerprint: strings.Repeat("b", 64)})
	if err != nil || result.Code != "trust_revoked" || store.revoked != strings.Repeat("b", 64) {
		t.Fatalf("RevokeTrust() = %+v, %v", result, err)
	}
}

func TestTrustRevocationRejectsActivePeer(t *testing.T) {
	store := &stubTrustStore{document: truststore.Document{
		SchemaVersion: truststore.SchemaVersion, Revision: 1, UpdatedAt: time.Now().UTC(),
		Peers: []truststore.PeerRecord{{
			Fingerprint: strings.Repeat("b", 64), Role: transport.AuthenticatedPeerConnector,
			Alias: "peer", CreatedAt: time.Now().UTC(),
		}},
	}}
	operations := testOperationsWithTrust(t, store)
	operations.mu.Lock()
	operations.peerAlias = "peer"
	operations.sessionDone = make(chan struct{})
	operations.mu.Unlock()
	result, err := operations.RevokeTrust(context.Background(), localapi.OperationRequest{Fingerprint: strings.Repeat("b", 64)})
	if err != nil || result.Code != "active_session_requires_disconnect" || store.revoked != "" {
		t.Fatalf("RevokeTrust() = %+v, %v", result, err)
	}
}

func testOperations(t *testing.T, runner SessionRunner) *Operations {
	t.Helper()
	operations, err := New(context.Background(), productidentity.PublicIdentity{
		SchemaVersion: productidentity.SchemaVersion,
		Fingerprint:   strings.Repeat("a", 64),
		Generation:    3,
		CreatedAt:     time.Now().UTC(),
	}, runner, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return operations
}

func testOperationsWithTrust(t *testing.T, trust trustStore) *Operations {
	t.Helper()
	operations, err := New(context.Background(), productidentity.PublicIdentity{
		SchemaVersion: productidentity.SchemaVersion,
		Fingerprint:   strings.Repeat("a", 64),
		Generation:    3,
		CreatedAt:     time.Now().UTC(),
	}, nil, nil, trust)
	if err != nil {
		t.Fatal(err)
	}
	return operations
}

type stubTrustStore struct {
	document truststore.Document
	revoked  string
}

func (store *stubTrustStore) List() (truststore.Document, error) { return store.document, nil }
func (store *stubTrustStore) Revoke(fingerprint string, _ time.Time) (truststore.PeerRecord, error) {
	store.revoked = fingerprint
	return store.document.Peers[0], nil
}
