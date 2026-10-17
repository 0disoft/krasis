package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func TestProductAuthorizerChecksCurrentPolicyAndCreatesFreshLeases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := newFlowNode(t, ctx)
	now := time.Now()
	identity, err := node.pairing.identities.OpenTransport(transport.AuthenticatedPeerConnector, now)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(identity.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	store := node.pairing.trust
	if _, err := store.ImportWithCertificate(peer, identity.Certificate[0], "peer", true, now); err != nil {
		t.Fatal(err)
	}
	policy, err := store.NewPeerPolicyMonitor(peer, now)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := productAuthorizer{policy}
	first, accepted, err := authorizer.AuthorizeLease(ctx, peer, "session", "epoch", now)
	if err != nil || !accepted || len(first) != 32 {
		t.Fatal("first lease", accepted, err)
	}
	second, accepted, err := authorizer.AuthorizeLease(ctx, peer, "session", "epoch", now)
	if err != nil || !accepted || first == second {
		t.Fatal("lease identifier was reused", accepted, err)
	}
	if _, err := store.Update(peer.CertificateSHA256, "peer", false, now); err != nil {
		t.Fatal(err)
	}
	if lease, accepted, err := authorizer.AuthorizeLease(ctx, peer, "session", "epoch", now); err != nil || accepted || lease != "" {
		t.Fatal("withdrawn grant accepted", accepted, err)
	}
	cancel()
	if _, _, err := (productAuthorizer{}).AuthorizeLease(ctx, peer, "session", "epoch", now); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request reached trust reader", err)
	}
}
