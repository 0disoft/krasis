package agent

import (
	"context"
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

func TestRuntimeEffectsRejectsDirectReleaseAll(t *testing.T) {
	if err := (RuntimeEffects{}).Execute(context.Background(), control.EffectReleaseAll); err == nil {
		t.Fatal("RuntimeEffects accepted direct ReleaseAll")
	}
}
