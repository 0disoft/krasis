package residentagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productcli"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

type flowNode struct {
	pairing    *networkPairing
	operations *Operations
	control    string
}

func TestReplacementCLIAndFinalRevocationGate(t *testing.T) {
	for _, revokeDuring := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "revoked_during_confirmation"}[revokeDuring], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			first, second, old := newFlowNode(t, ctx), newFlowNode(t, ctx), newFlowNode(t, ctx)
			certificate, err := old.pairing.identities.OpenTransport(transport.AuthenticatedPeerConnector, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			oldPeer, err := transport.AuthenticatedPeerFromCertificate(certificate.Leaf)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := first.pairing.trust.ImportWithCertificate(oldPeer, certificate.Certificate[0], "old", true, time.Now()); err != nil {
				t.Fatal(err)
			}
			client, err := localapi.OpenClient(first.control)
			if err != nil {
				t.Fatal(err)
			}
			allow := true
			request := localapi.OperationRequest{PairingAction: "LISTEN", PeerAlias: "new", Address: "127.0.0.1:12345", Fingerprint: oldPeer.CertificateSHA256, AllowInboundControl: &allow}
			first.operations.runner = func(ctx context.Context, _ string, _ agent.SupervisorObserver) error { <-ctx.Done(); return ctx.Err() }
			flowCLI(t, first, "connect", "old")
			if result, err := client.Pair(ctx, request); err != nil || result.Code != "active_session_requires_disconnect" {
				t.Fatalf("active replacement: %+v %v", result, err)
			}
			flowCLI(t, first, "disconnect")
			invalid := request
			invalid.Fingerprint = "bad"
			if _, err := client.Pair(ctx, invalid); err == nil {
				t.Fatal("API accepted malformed fingerprint")
			}
			invalid.Fingerprint = strings.Repeat("0", 64)
			if result, err := client.Pair(ctx, invalid); err != nil || result.Code != "peer_not_found" {
				t.Fatalf("unknown target: %+v %v", result, err)
			}
			invalid = request
			invalid.PairingAction = "DIAL"
			if result, err := client.Pair(ctx, invalid); err != nil || result.Code != "replacement_role_mismatch" {
				t.Fatalf("wrong role: %+v %v", result, err)
			}
			var output bytes.Buffer
			if exit := productcli.Run(ctx, []string{"--control-dir", first.control, "pair", "listen", "new", "127.0.0.1:12345", "--allow-inbound-control", "--replace", "bad"}, &output, &output, "unused"); exit != productcli.ExitUsage {
				t.Fatalf("invalid CLI fingerprint exit=%d", exit)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			first.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
			started := flowCLI(t, first, "pair", "listen", "new", listener.Addr().String(), "--allow-inbound-control", "--replace", oldPeer.CertificateSHA256)
			remote := flowCLI(t, second, "pair", "dial", "first", listener.Addr().String(), "--allow-inbound-control")
			job, remoteJob := waitPairReady(t, first.pairing), waitPairReady(t, second.pairing)
			status := flowCLI(t, first, "pair", "status", started.PairingID)
			if _, err := client.Pair(ctx, localapi.OperationRequest{PairingAction: "CONFIRM", PairingID: started.PairingID, ConfirmationCode: status.Confirmation, Fingerprint: oldPeer.CertificateSHA256}); err == nil {
				t.Fatal("confirm accepted a replacement target override")
			}
			if revokeDuring {
				if _, err := first.pairing.trust.Revoke(oldPeer.CertificateSHA256, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			flowCLI(t, first, "pair", "confirm", started.PairingID, status.Confirmation)
			if !revokeDuring {
				if _, err := first.pairing.trust.PeerCertificate(oldPeer, time.Now()); err != nil {
					t.Fatalf("one-sided confirmation replaced trust: %v", err)
				}
			}
			flowCLI(t, second, "pair", "confirm", remote.PairingID, status.Confirmation)
			for _, finished := range []*pairJob{job, remoteJob} {
				select {
				case <-finished.done:
				case <-time.After(5 * time.Second):
					t.Fatal("replacement did not finish")
				}
			}
			result, err := client.Pair(ctx, localapi.OperationRequest{PairingAction: "STATUS", PairingID: started.PairingID})
			if err != nil {
				t.Fatal(err)
			}
			want := "pairing_completed"
			if revokeDuring {
				want = "pairing_commit_uncertain"
			}
			if result.Code != want {
				t.Fatalf("replacement status=%+v", result)
			}
			// Re-read after ignoring the confirm response: status is observational
			// and must retain the local outcome, including one-sided failure.
			again, err := client.Pair(ctx, localapi.OperationRequest{PairingAction: "STATUS", PairingID: started.PairingID})
			if err != nil || again != result {
				t.Fatalf("repeat status=%+v %v; initial=%+v", again, err, result)
			}
			remoteStatus := flowCLI(t, second, "pair", "status", remote.PairingID)
			if remoteStatus.Code != "pairing_completed" {
				t.Fatalf("remote local commit=%+v", remoteStatus)
			}
			document, err := first.pairing.trust.List()
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 2
			if revokeDuring {
				wantCount = 1
			}
			if len(document.Peers) != wantCount {
				t.Fatalf("unexpected peers: %d", len(document.Peers))
			}
			if _, err := first.pairing.trust.PeerCertificate(oldPeer, time.Now()); !errors.Is(err, truststore.ErrPeerRevoked) {
				t.Fatalf("old peer not revoked: %v", err)
			}
			if result, err := client.Pair(ctx, request); err != nil || result.Code != "peer_already_revoked" {
				t.Fatalf("revoked target accepted: %+v %v", result, err)
			}
			if !revokeDuring {
				for _, peer := range document.Peers {
					if peer.RevokedAt == nil {
						if _, err := first.pairing.trust.PeerCertificate(transport.AuthenticatedPeerContext{CertificateSHA256: peer.Fingerprint, Role: peer.Role}, time.Now()); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		})
	}
}

func newFlowNode(t *testing.T, ctx context.Context) flowNode {
	t.Helper()
	paths, identity, _, err := productidentity.BootstrapWindows(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	identities, err := productidentity.New(paths.Identity)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := truststore.New(paths.Trust)
	if err != nil {
		t.Fatal(err)
	}
	pair := &networkPairing{root: ctx, identity: identity, identities: identities, trust: trust}
	operations, err := New(ctx, identity.Public(), nil, pair, trust)
	if err != nil {
		t.Fatal(err)
	}
	operations.credentials = identities
	server, err := localapi.Listen(paths.Control, operations)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := operations.Close(cleanup); err != nil {
			t.Error(err)
		}
		if err := server.Close(cleanup); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return flowNode{pair, operations, paths.Control}
}

func TestCredentialCLIInspectionRenewalAndGuards(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := newFlowNode(t, ctx)
	client, err := localapi.OpenClient(node.control)
	if err != nil {
		t.Fatal(err)
	}
	role := transport.AuthenticatedPeerListener
	request := localapi.CredentialRequest{Action: "SHOW", Role: role}
	result, err := client.Credential(ctx, request)
	if err != nil || result.Code != "credential_not_found" {
		t.Fatalf("empty inspection: %+v %v", result, err)
	}
	old, err := node.pairing.identities.OpenTransport(role, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(old.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	run := func(wantExit int, args ...string) localapi.CredentialResult {
		t.Helper()
		var stdout, stderr bytes.Buffer
		exit := productcli.Run(ctx, append([]string{"--json", "--control-dir", node.control}, args...), &stdout, &stderr, "unused")
		if exit != wantExit {
			t.Fatalf("credential CLI exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
		}
		if strings.Contains(stdout.String(), "privateKey") || strings.Contains(stdout.String(), "certificate\"") {
			t.Fatal("credential material in output")
		}
		var value localapi.CredentialResult
		if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	shown := run(productcli.ExitOK, "credential", "show", "LISTENER")
	if shown.Info == nil || shown.Info.Fingerprint != peer.CertificateSHA256 {
		t.Fatal("wrong public metadata")
	}
	node.operations.runner = func(ctx context.Context, _ string, _ agent.SupervisorObserver) error { <-ctx.Done(); return ctx.Err() }
	flowCLI(t, node, "connect", "peer")
	if result := run(productcli.ExitUnsafeState, "credential", "renew", "LISTENER", peer.CertificateSHA256); result.Code != "active_session_requires_disconnect" {
		t.Fatalf("active renewal: %+v", result)
	}
	flowCLI(t, node, "disconnect")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	node.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
	started := flowCLI(t, node, "pair", "listen", "peer", listener.Addr().String(), "--allow-inbound-control")
	if result := run(productcli.ExitUnsafeState, "credential", "renew", "LISTENER", peer.CertificateSHA256); result.Code != "pairing_already_active" {
		t.Fatalf("pairing renewal: %+v", result)
	}
	flowCLI(t, node, "pair", "cancel", started.PairingID)
	node.pairing.mu.Lock()
	job := node.pairing.job
	node.pairing.mu.Unlock()
	select {
	case <-job.done:
	case <-time.After(time.Second):
		t.Fatal("pair did not stop")
	}
	// Expiry is simulated at the agent clock; no wall-clock waiting is required.
	node.operations.now = func() time.Time { return old.Leaf.NotAfter }
	if expired := run(productcli.ExitOK, "credential", "show", "LISTENER"); expired.Info == nil || !expired.Info.Expired {
		t.Fatal("expiry not visible")
	}
	renewed := run(productcli.ExitOK, "credential", "renew", "LISTENER", peer.CertificateSHA256)
	if renewed.Code != "credential_renewed_repair_required" || renewed.Info == nil || renewed.Info.Fingerprint == peer.CertificateSHA256 {
		t.Fatalf("renewal: %+v", renewed)
	}
	if stale := run(productcli.ExitUnsafeState, "credential", "renew", "LISTENER", peer.CertificateSHA256); stale.Code != "credential_changed" {
		t.Fatalf("stale retry: %+v", stale)
	}
	if _, err := node.pairing.trust.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
		t.Fatalf("renewal granted trust: %v", err)
	}
}

func flowCLI(t *testing.T, node flowNode, args ...string) localapi.OperationResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := productcli.Run(context.Background(), append([]string{"--json", "--control-dir", node.control}, args...), &stdout, &stderr, "unused")
	if code != productcli.ExitOK {
		t.Fatalf("CLI %v exit=%d stdout=%s stderr=%s", args, code, stdout.String(), stderr.String())
	}
	var result localapi.OperationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func waitPairReady(t *testing.T, service *networkPairing) *pairJob {
	t.Helper()
	service.mu.Lock()
	job := service.job
	service.mu.Unlock()
	select {
	case <-job.ready:
	case <-job.done:
		t.Fatal("pairing failed before confirmation")
	case <-time.After(5 * time.Second):
		t.Fatal("pairing did not become ready")
	}
	return job
}

func TestProductCLIPairsNewDevicesAndPreparedTransportConnects(t *testing.T) {
	for _, both := range []bool{false, true} {
		t.Run(map[bool]string{false: "one_sided_renewal", true: "both_sides_renewal"}[both], func(t *testing.T) { productRenewalFlow(t, both, true) })
	}
	t.Run("deny_inbound_reconfirmation", func(t *testing.T) { productRenewalFlow(t, false, false) })
}

func productRenewalFlow(t *testing.T, both, allowInbound bool) {
	option := "--allow-inbound-control"
	if !allowInbound {
		option = "--deny-inbound-control"
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := newFlowNode(t, ctx), newFlowNode(t, ctx)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	first.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
	firstStart := flowCLI(t, first, "pair", "listen", "second", listener.Addr().String(), option)
	secondStart := flowCLI(t, second, "pair", "dial", "first", listener.Addr().String(), option)
	firstJob, secondJob := waitPairReady(t, first.pairing), waitPairReady(t, second.pairing)
	firstStatus := flowCLI(t, first, "pair", "status", firstStart.PairingID)
	secondStatus := flowCLI(t, second, "pair", "status", secondStart.PairingID)
	if firstStatus.Confirmation == "" || firstStatus.Confirmation != secondStatus.Confirmation {
		t.Fatal("confirmation codes differ")
	}
	flowCLI(t, first, "pair", "confirm", firstStart.PairingID, firstStatus.Confirmation)
	if _, err := first.pairing.trust.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
		t.Fatalf("one-sided confirmation granted trust: %v", err)
	}
	flowCLI(t, second, "pair", "confirm", secondStart.PairingID, secondStatus.Confirmation)
	for _, job := range []*pairJob{firstJob, secondJob} {
		select {
		case <-job.done:
		case <-time.After(5 * time.Second):
			t.Fatal("pairing did not finish")
		}
	}
	for _, item := range []struct {
		node flowNode
		id   string
	}{{first, firstStart.PairingID}, {second, secondStart.PairingID}} {
		if result := flowCLI(t, item.node, "pair", "status", item.id); result.Code != "pairing_completed" {
			t.Fatalf("pairing result=%+v", result)
		}
	}
	if _, err := first.pairing.trust.SetConnection("second", truststore.ConnectionSettings{Address: "127.0.0.1:12345", Edge: "RIGHT"}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.pairing.trust.SetConnection("first", truststore.ConnectionSettings{Address: "127.0.0.1:12345", Edge: "LEFT"}); err != nil {
		t.Fatal(err)
	}
	serverConfig, err := prepareProductConnection(first.pairing.identities, first.pairing.trust, "second")
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err := prepareProductConnection(second.pairing.identities, second.pairing.trust, "first")
	if err != nil {
		t.Fatal(err)
	}
	source, err := agent.NewListenSource("127.0.0.1:0", serverConfig.tls)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dial, err := agent.NewDialSource(source.Address(), clientConfig.tls, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	handshake, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	type acceptedPeer struct {
		connection agent.Connection
		err        error
	}
	accepted := make(chan acceptedPeer, 1)
	go func() {
		conn, err := source.Next(handshake)
		accepted <- acceptedPeer{conn, err}
	}()
	conn, err := dial.Next(handshake)
	// Server-side completion does not prove the client has consumed and
	// verified the final TLS flight. Keep both endpoints until both finish.
	serverPeer := <-accepted
	if serverPeer.connection != nil {
		defer serverPeer.connection.Close()
	}
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	if serverPeer.err != nil {
		t.Fatal(serverPeer.err)
	}
	conn.Close()
	serverPeer.connection.Close()
	oldFirst, oldSecond := clientConfig.peer, serverConfig.peer
	beforeReconfirm, err := first.pairing.trust.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeReconfirm.Peers) != 1 || beforeReconfirm.Peers[0].AllowInboundControl != allowInbound {
		t.Fatal("pairing stored wrong inbound permission")
	}
	flowCLI(t, first, "credential", "renew", "LISTENER", oldFirst.CertificateSHA256)
	if both {
		flowCLI(t, second, "credential", "renew", "CONNECTOR", oldSecond.CertificateSHA256)
	}
	newListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer newListener.Close()
	first.pairing.listen = func(string, string) (net.Listener, error) { return newListener, nil }
	args := []string{"pair", "listen", "second", newListener.Addr().String(), option}
	if both {
		args = append(args, "--replace", oldSecond.CertificateSHA256)
	}
	firstStart = flowCLI(t, first, args...)
	secondStart = flowCLI(t, second, "pair", "dial", "first", newListener.Addr().String(), option, "--replace", oldFirst.CertificateSHA256)
	firstJob, secondJob = waitPairReady(t, first.pairing), waitPairReady(t, second.pairing)
	firstStatus = flowCLI(t, first, "pair", "status", firstStart.PairingID)
	secondStatus = flowCLI(t, second, "pair", "status", secondStart.PairingID)
	if firstStatus.Confirmation != secondStatus.Confirmation {
		t.Fatal("renewal confirmation differs")
	}
	flowCLI(t, first, "pair", "confirm", firstStart.PairingID, firstStatus.Confirmation)
	flowCLI(t, second, "pair", "confirm", secondStart.PairingID, secondStatus.Confirmation)
	for _, job := range []*pairJob{firstJob, secondJob} {
		select {
		case <-job.done:
		case <-time.After(5 * time.Second):
			t.Fatal("renewal re-pair did not finish")
		}
	}
	if flowCLI(t, first, "pair", "status", firstStart.PairingID).Code != "pairing_completed" || flowCLI(t, second, "pair", "status", secondStart.PairingID).Code != "pairing_completed" {
		t.Fatal("renewal replacement failed")
	}
	if !both {
		after, err := first.pairing.trust.List()
		if err != nil || !reflect.DeepEqual(beforeReconfirm, after) {
			t.Fatalf("reconfirmation changed trust: %v", err)
		}
		profile, err := first.pairing.trust.Connection("second")
		if err != nil || profile != serverConfig.profile {
			t.Fatalf("reconfirmation changed connection: %v", err)
		}
	}
	if _, err := first.pairing.trust.SetConnection("second", serverConfig.profile.ConnectionSettings); err != nil {
		t.Fatal(err)
	}
	if _, err := second.pairing.trust.SetConnection("first", clientConfig.profile.ConnectionSettings); err != nil {
		t.Fatal(err)
	}
	serverConfig, err = prepareProductConnection(first.pairing.identities, first.pairing.trust, "second")
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err = prepareProductConnection(second.pairing.identities, second.pairing.trust, "first")
	if err != nil {
		t.Fatal(err)
	}
	if (both && serverConfig.peer == oldSecond) || (!both && serverConfig.peer != oldSecond) || clientConfig.peer == oldFirst {
		t.Fatal("renewal reused old peer")
	}
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	renewalContext, stopRenewal := context.WithTimeout(ctx, 3*time.Second)
	defer stopRenewal()
	renewalServer := make(chan error, 1)
	go func() {
		actual, err := transport.AuthenticatePeerContext(renewalContext, tls.Server(left, serverConfig.tls))
		if err == nil && actual != serverConfig.peer {
			err = errors.New("wrong renewed connector")
		}
		renewalServer <- err
	}()
	actual, renewedErr := transport.AuthenticatePeerContext(renewalContext, tls.Client(right, clientConfig.tls))
	serverErr := <-renewalServer
	if renewedErr != nil || serverErr != nil || actual != clientConfig.peer {
		t.Fatalf("renewed TLS: client=%v server=%v", renewedErr, serverErr)
	}
	if !allowInbound {
		for _, check := range []struct {
			store *truststore.Store
			peer  transport.AuthenticatedPeerContext
		}{{first.pairing.trust, serverConfig.peer}, {second.pairing.trust, clientConfig.peer}} {
			lease, allowed, err := (productAuthorizer{check.store}).AuthorizeLease(ctx, check.peer, "session", "epoch", time.Now())
			if err != nil || allowed || lease != "" {
				t.Fatalf("deny option granted input: allowed=%t err=%v", allowed, err)
			}
		}
	}
	if _, err := second.pairing.trust.PeerCertificate(oldFirst, time.Now()); !errors.Is(err, truststore.ErrPeerRevoked) {
		t.Fatal("old renewed peer remained trusted")
	}
	if _, err := second.pairing.trust.Revoke(clientConfig.peer.CertificateSHA256, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareProductConnection(second.pairing.identities, second.pairing.trust, "first"); err == nil {
		t.Fatal("revoked peer remained connectable")
	}
}

func TestPairCancelStopsListenerAndNeverImportsTrust(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := newFlowNode(t, ctx)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	node.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
	started := flowCLI(t, node, "pair", "listen", "peer", listener.Addr().String(), "--allow-inbound-control")
	node.pairing.mu.Lock()
	job := node.pairing.job
	node.pairing.mu.Unlock()
	flowCLI(t, node, "pair", "cancel", started.PairingID)
	select {
	case <-job.done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not stop pending listener")
	}
	if _, err := node.pairing.trust.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
		t.Fatalf("cancel registered trust: %v", err)
	}
}

func TestPairWrongConfirmationNeverImportsTrust(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
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
	wrong := "000000"
	if status.Confirmation == wrong {
		wrong = "111111"
	}
	result, err := first.pairing.Handle(ctx, localapi.OperationRequest{PairingAction: "CONFIRM", PairingID: started.PairingID, ConfirmationCode: wrong})
	if err != nil || result.Code != "confirmation_rejected" {
		t.Fatalf("wrong confirmation: %+v %v", result, err)
	}
	for _, action := range []string{"LISTEN", "DIAL"} {
		result, err := first.pairing.Handle(ctx, localapi.OperationRequest{PairingAction: action, PeerAlias: "different-peer"})
		if err != nil || result.Code != "pairing_retry_later" {
			t.Fatalf("retry bypass: %+v %v", result, err)
		}
	}
	select {
	case <-job.done:
	case <-time.After(time.Second):
		t.Fatal("wrong confirmation did not stop pairing")
	}
	for _, node := range []flowNode{first, second} {
		if _, err := node.pairing.trust.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
			t.Fatalf("wrong confirmation imported trust: %v", err)
		}
	}
}

func TestPairingDeadlineCannotBeExtendedByStatusOrConfirmation(t *testing.T) {
	cancelled := false
	job := &pairJob{id: "job", code: "confirmation_required", confirmation: "123456", expiresAt: time.Now().Add(-time.Second), done: make(chan struct{}), cancel: func() { cancelled = true }}
	service := &networkPairing{root: context.Background(), job: job}
	result, err := service.Handle(context.Background(), localapi.OperationRequest{PairingAction: "STATUS", PairingID: job.id})
	if err != nil || result.Code != "pairing_expired" || result.Confirmation != "" || !cancelled {
		t.Fatalf("expiry: %+v %v", result, err)
	}
	result, err = service.Handle(context.Background(), localapi.OperationRequest{PairingAction: "CONFIRM", PairingID: job.id, ConfirmationCode: "123456"})
	if err != nil || result.Code != "pairing_finished" {
		t.Fatalf("expired confirm: %+v %v", result, err)
	}
	service.nextStart = time.Now().Add(-time.Second)
	result, err = service.Handle(context.Background(), localapi.OperationRequest{PairingAction: "DIAL"})
	if err != nil || result.Code != "invalid_request" {
		t.Fatalf("elapsed cooldown: %+v %v", result, err)
	}
}
