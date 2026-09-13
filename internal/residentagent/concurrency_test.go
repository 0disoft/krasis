package residentagent

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestConcurrentPairConfirmCancelNeverImportsTrust(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, second := newFlowNode(t, ctx), newFlowNode(t, ctx)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	first.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
	started := flowCLI(t, first, "pair", "listen", "second", listener.Addr().String(), "--allow-inbound-control")
	flowCLI(t, second, "pair", "dial", "first", listener.Addr().String(), "--allow-inbound-control")
	job := waitPairReady(t, first.pairing)
	waitPairReady(t, second.pairing)
	status := flowCLI(t, first, "pair", "status", started.PairingID)
	start := make(chan struct{})
	results := make(chan localapi.OperationResult, 2)
	for _, action := range []string{"CONFIRM", "CANCEL"} {
		go func(action string) {
			<-start
			result, err := first.pairing.Handle(ctx, localapi.OperationRequest{PairingAction: action, PairingID: started.PairingID, ConfirmationCode: status.Confirmation})
			if err != nil {
				t.Errorf("%s: %v", action, err)
			}
			results <- result
		}(action)
	}
	close(start)
	for range 2 {
		select {
		case result := <-results:
			if result.Code != "confirmation_submitted" && result.Code != "pairing_cancelled" && result.Code != "pairing_finished" {
				t.Errorf("unexpected outcome: %+v", result)
			}
		case <-ctx.Done():
			t.Fatal("concurrent requests did not finish")
		}
	}
	select {
	case <-job.done:
	case <-ctx.Done():
		t.Fatal("cancelled job did not stop")
	}
	if result := flowCLI(t, first, "pair", "status", started.PairingID); result.Code != "pairing_cancelled" {
		t.Fatalf("terminal outcome: %+v", result)
	}
	// The other device never confirmed. Neither ordering may create local trust.
	if _, err := first.pairing.trust.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
		t.Fatalf("unexpected trust: %v", err)
	}
}

func TestConcurrentCredentialRenewAndConnectUseOneIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	node := newFlowNode(t, ctx)
	role := transport.AuthenticatedPeerListener
	if _, err := node.pairing.identities.OpenTransport(role, time.Now()); err != nil {
		t.Fatal(err)
	}
	old, err := node.pairing.identities.InspectTransport(role, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	observed := make(chan string, 1)
	node.operations.preflight = func(string) error {
		info, err := node.pairing.identities.InspectTransport(role, time.Now())
		if err == nil {
			observed <- info.Fingerprint
		}
		return err
	}
	node.operations.runner = func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
		<-ctx.Done()
		return context.Cause(ctx)
	}
	start := make(chan struct{})
	connected := make(chan localapi.OperationResult, 1)
	renewed := make(chan localapi.CredentialResult, 1)
	go func() {
		<-start
		result, err := node.operations.Connect(ctx, localapi.OperationRequest{PeerAlias: "peer"})
		if err != nil {
			t.Errorf("connect: %v", err)
		}
		connected <- result
	}()
	go func() {
		<-start
		result, err := node.operations.Credential(ctx, localapi.CredentialRequest{SchemaVersion: localapi.CredentialSchemaVersion, Action: "RENEW", Role: role, ExpectedFingerprint: old.Fingerprint})
		if err != nil {
			t.Errorf("renew: %v", err)
		}
		renewed <- result
	}()
	close(start)
	var renewal localapi.CredentialResult
	select {
	case result := <-connected:
		if result.Code != "connect_started" {
			t.Fatalf("connect: %+v", result)
		}
	case <-ctx.Done():
		t.Fatal("connect blocked")
	}
	select {
	case renewal = <-renewed:
	case <-ctx.Done():
		t.Fatal("renew blocked")
	}
	current, err := node.pairing.identities.InspectTransport(role, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case fingerprint := <-observed:
		if fingerprint != current.Fingerprint {
			t.Fatal("identity changed after connection preflight")
		}
	case <-ctx.Done():
		t.Fatal("preflight missing")
	}
	switch renewal.Code {
	case "credential_renewed_repair_required":
		if renewal.Info == nil || renewal.Info.Fingerprint != current.Fingerprint || current.Fingerprint == old.Fingerprint {
			t.Fatal("renewal was not persisted before connect")
		}
	case "active_session_requires_disconnect":
		if current.Fingerprint != old.Fingerprint {
			t.Fatal("rejected renewal changed identity")
		}
	default:
		t.Fatalf("renew: %+v", renewal)
	}
	if result, err := node.operations.Disconnect(ctx, localapi.OperationRequest{}); err != nil || result.Code != "disconnected" {
		t.Fatalf("disconnect: %+v %v", result, err)
	}
}
