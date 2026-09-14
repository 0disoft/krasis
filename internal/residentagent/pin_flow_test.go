package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/truststore"
	"io"
	"net"
	"testing"
	"time"
)

type invitationCloser struct{}

func (invitationCloser) Close() error { return nil }

func TestPINRegistrationOverAuthenticatedLocalAPI(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "wrong"}[wrong], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, b := newFlowNode(t, ctx), newFlowNode(t, ctx)
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			a.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
			a.pairing.advertise = func(context.Context, string, int) (io.Closer, error) { return invitationCloser{}, nil }
			ca, err := localapi.OpenClient(a.control)
			if err != nil {
				t.Fatal(err)
			}
			cb, err := localapi.OpenClient(b.control)
			if err != nil {
				t.Fatal(err)
			}
			allow := true
			invite, err := ca.Pair(ctx, localapi.OperationRequest{PairingAction: "PIN_LISTEN", PeerAlias: "right", AllowInboundControl: &allow})
			if err != nil || invite.Code != "pin_waiting" || len(invite.Confirmation) != 6 {
				t.Fatalf("invitation: %s %v", invite.Code, err)
			}
			pin := invite.Confirmation
			manual, err := ca.Pair(ctx, localapi.OperationRequest{PairingAction: "CONFIRM", PairingID: invite.PairingID, ConfirmationCode: pin})
			if err != nil || manual.Code != "invalid_request" {
				t.Fatal("PIN invitation accepted a legacy confirmation")
			}
			if wrong {
				if pin == "000000" {
					pin = "111111"
				} else {
					pin = "000000"
				}
			}
			joined, err := cb.Pair(ctx, localapi.OperationRequest{PairingAction: "PIN_DIAL", PeerAlias: "left", AllowInboundControl: &allow, Address: listener.Addr().String(), PairingID: invite.PairingID, ConfirmationCode: pin})
			if err != nil || joined.Code != "pin_waiting" {
				t.Fatalf("join: %s %v", joined.Code, err)
			}
			for _, node := range []flowNode{a, b} {
				node.pairing.mu.Lock()
				job := node.pairing.job
				node.pairing.mu.Unlock()
				select {
				case <-job.done:
				case <-time.After(3 * time.Second):
					t.Fatal("PIN exchange stalled")
				}
				node.pairing.mu.Lock()
				code, secret := job.code, job.pin
				node.pairing.mu.Unlock()
				if secret != "" {
					t.Fatal("finished job retained PIN")
				}
				if wrong {
					if code != "pairing_failed" {
						t.Fatal("wrong PIN did not fail")
					}
					if _, err := node.pairing.trust.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
						t.Fatal("wrong PIN wrote trust")
					}
				} else {
					if code != "pairing_completed" {
						t.Fatalf("registration failed: %s", code)
					}
					profile, err := node.pairing.trust.Connection(job.alias)
					if err != nil || profile.Address != "127.0.0.1:45872" {
						t.Fatalf("connection profile: %v", err)
					}
					want := "LEFT"
					if node.pairing == a.pairing {
						want = "RIGHT"
					}
					if profile.Edge != want {
						t.Fatal("wrong screen edge")
					}
				}
			}
		})
	}
}
