package localapi

import (
	"strings"
	"testing"
)

func TestPINRequestsKeepConsentAndSecretsScoped(t *testing.T) {
	allow := true
	invite := OperationRequest{PairingAction: "PIN_LISTEN", PeerAlias: "peer", AllowInboundControl: &allow}
	if validateOperationRequest("pair", invite) != nil {
		t.Fatal("valid invite rejected")
	}
	invite.ConfirmationCode = "123456"
	if validateOperationRequest("pair", invite) == nil {
		t.Fatal("caller-selected invitation PIN accepted")
	}
	join := OperationRequest{PairingAction: "PIN_DIAL", PeerAlias: "peer", AllowInboundControl: &allow, Address: "192.168.1.7:45871", PairingID: strings.Repeat("a", 32), ConfirmationCode: "012345"}
	if validateOperationRequest("pair", join) != nil {
		t.Fatal("valid join rejected")
	}
	join.AllowInboundControl = nil
	if validateOperationRequest("pair", join) == nil {
		t.Fatal("missing consent accepted")
	}
	join.AllowInboundControl = &allow
	join.PairingAction = "DIAL"
	if validateOperationRequest("pair", join) == nil {
		t.Fatal("PIN leaked into legacy path")
	}
}
