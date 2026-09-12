package residentagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
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
				want = "pairing_failed"
			}
			if result.Code != want {
				t.Fatalf("replacement status=%+v", result)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := newFlowNode(t, ctx), newFlowNode(t, ctx)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	first.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
	firstStart := flowCLI(t, first, "pair", "listen", "second", listener.Addr().String(), "--allow-inbound-control")
	secondStart := flowCLI(t, second, "pair", "dial", "first", listener.Addr().String(), "--allow-inbound-control")
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
	accepted := make(chan error, 1)
	go func() {
		conn, err := source.Next(handshake)
		if conn != nil {
			conn.Close()
		}
		accepted <- err
	}()
	conn, err := dial.Next(handshake)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if err := <-accepted; err != nil {
		t.Fatal(err)
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
