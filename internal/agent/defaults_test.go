package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestAuthenticatedPeerAuthorizerCreatesFreshLeaseIdentifiers(t *testing.T) {
	authorizer := AuthenticatedPeerAuthorizer{}
	first, accepted, err := authorizer.AuthorizeLease(context.Background(), testAuthenticatedPeerContext(), "session", "epoch", time.Now())
	if err != nil || !accepted || len(first) != leaseIdentifierBytes*2 {
		t.Fatalf("first authorization = %q, %t, %v", first, accepted, err)
	}
	second, accepted, err := authorizer.AuthorizeLease(context.Background(), testAuthenticatedPeerContext(), "session", "epoch", time.Now())
	if err != nil || !accepted || first == second {
		t.Fatalf("second authorization = %q, %t, %v", second, accepted, err)
	}
}

func TestAuthenticatedPeerAuthorizerRejectsMissingPeerIdentity(t *testing.T) {
	leaseID, accepted, err := (AuthenticatedPeerAuthorizer{}).AuthorizeLease(
		context.Background(),
		transport.AuthenticatedPeerContext{},
		"session",
		"epoch",
		time.Now(),
	)
	if err == nil || accepted || leaseID != "" {
		t.Fatalf("authorization = %q, %t, %v", leaseID, accepted, err)
	}
}

func TestTrustedPeerAuthorizerEnforcesFingerprintRolePermissionAndRevocation(t *testing.T) {
	trusted := testAuthenticatedPeerContext()
	tests := []struct {
		name     string
		grant    TrustedPeerGrant
		peer     transport.AuthenticatedPeerContext
		accepted bool
	}{
		{name: "trusted", grant: TrustedPeerGrant{Peer: trusted, AllowInboundControl: true}, peer: trusted, accepted: true},
		{name: "permission denied", grant: TrustedPeerGrant{Peer: trusted}, peer: trusted},
		{name: "revoked", grant: TrustedPeerGrant{Peer: trusted, AllowInboundControl: true, Revoked: true}, peer: trusted},
		{name: "unknown fingerprint", grant: TrustedPeerGrant{Peer: trusted, AllowInboundControl: true}, peer: transport.AuthenticatedPeerContext{CertificateSHA256: strings.Repeat("c", 64), Role: trusted.Role}},
		{name: "wrong role", grant: TrustedPeerGrant{Peer: trusted, AllowInboundControl: true}, peer: transport.AuthenticatedPeerContext{CertificateSHA256: trusted.CertificateSHA256, Role: transport.AuthenticatedPeerListener}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorizer, err := NewTrustedPeerAuthorizer([]TrustedPeerGrant{test.grant})
			if err != nil {
				t.Fatal(err)
			}
			leaseID, accepted, err := authorizer.AuthorizeLease(context.Background(), test.peer, "session", "epoch", time.Now())
			if err != nil || accepted != test.accepted || (accepted && leaseID == "") || (!accepted && leaseID != "") {
				t.Fatalf("authorization = %q, %t, %v", leaseID, accepted, err)
			}
		})
	}
}

func TestTrustedPeerAuthorizerRejectsDuplicateFingerprint(t *testing.T) {
	peer := testAuthenticatedPeerContext()
	_, err := NewTrustedPeerAuthorizer([]TrustedPeerGrant{{Peer: peer}, {Peer: peer}})
	if err == nil {
		t.Fatal("duplicate trusted peer fingerprint was accepted")
	}
}

func TestRuntimeEffectsRejectsDirectReleaseAll(t *testing.T) {
	if err := (RuntimeEffects{}).Execute(context.Background(), control.EffectReleaseAll); err == nil {
		t.Fatal("RuntimeEffects accepted direct ReleaseAll")
	}
}
