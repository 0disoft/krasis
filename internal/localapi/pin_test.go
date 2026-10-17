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

func TestPINAliasPrefixValidation(t *testing.T) {
	allow := true
	for _, prefix := range []string{"", " ", "peer ", " peer", "peer\n", string([]byte{0xff}), strings.Repeat("가", 56)} {
		for _, action := range []string{"PIN_LISTEN", "PIN_DIAL"} {
			request := OperationRequest{PairingAction: action, PeerAlias: prefix, AllowInboundControl: &allow,
				Address: "192.168.1.7:45871", PairingID: strings.Repeat("a", 32), ConfirmationCode: "012345"}
			if action == "PIN_LISTEN" {
				request.Address, request.PairingID, request.ConfirmationCode = "", "", ""
			}
			if validateOperationRequest("pair", request) == nil {
				t.Fatalf("invalid prefix %q accepted for %s", prefix, action)
			}
		}
	}
	for _, prefix := range []string{"peer", "새 노트북", strings.Repeat("가", 55)} {
		if !validPINAliasPrefix(prefix) {
			t.Fatalf("valid prefix %q rejected", prefix)
		}
	}
}
