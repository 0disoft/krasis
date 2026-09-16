package productpanel

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type fakeClient struct {
	state                 string
	peers                 []localapi.TrustedPeer
	connects, disconnects int
	err, trustErr         error
}

func (c *fakeClient) Status(context.Context) (localapi.Status, error) {
	return localapi.Status{State: c.state}, c.err
}
func (c *fakeClient) Trust(context.Context) (localapi.Trust, error) {
	return localapi.Trust{Peers: c.peers}, c.trustErr
}
func (c *fakeClient) Connect(context.Context, string) (localapi.OperationResult, error) {
	c.connects++
	c.state = "CONNECTING"
	return localapi.OperationResult{Status: "COMPLETED"}, nil
}
func (c *fakeClient) Disconnect(context.Context) (localapi.OperationResult, error) {
	c.disconnects++
	c.state = "LOCAL"
	return localapi.OperationResult{Status: "COMPLETED"}, nil
}

func TestPanelFiltersPeersAndUsesAgentState(t *testing.T) {
	c := &fakeClient{state: "LOCAL", peers: []localapi.TrustedPeer{{Alias: "B"}, {Alias: "revoked", Revoked: true}, {Alias: "duplicate"}, {Alias: "duplicate"}}}
	open := func() (Client, error) { return c, nil }
	view := Execute(context.Background(), open, "refresh", "")
	if !reflect.DeepEqual(view.Peers, []string{"B"}) || c.connects != 0 {
		t.Fatalf("refresh: %+v", view)
	}
	Execute(context.Background(), open, "connect", "revoked")
	Execute(context.Background(), open, "connect", "duplicate")
	if c.connects != 0 {
		t.Fatal("unselectable peer connected")
	}
	view = Execute(context.Background(), open, "connect", "B")
	if c.connects != 1 || view.State != "CONNECTING" {
		t.Fatalf("invented connected state: %+v", view)
	}
	Execute(context.Background(), open, "connect", "B")
	if c.connects != 1 {
		t.Fatal("duplicate connect")
	}
	view = Execute(context.Background(), open, "disconnect", "")
	if c.disconnects != 1 || view.State != "LOCAL" {
		t.Fatalf("disconnect: %+v", view)
	}
}

func TestPanelRedactsFailuresAndDoesNotConnect(t *testing.T) {
	secret := errors.New("secret-token private-path")
	c := &fakeClient{err: secret}
	for _, open := range []func() (Client, error){func() (Client, error) { return nil, secret }, func() (Client, error) { return c, nil }} {
		view := Execute(context.Background(), open, "connect", "B")
		if view.State != "" || strings.Contains(view.Notice, "secret") || c.connects != 0 {
			t.Fatalf("unsafe failure: %+v", view)
		}
	}
}

func TestDisconnectDoesNotDependOnTrustRead(t *testing.T) {
	c := &fakeClient{state: "CONNECTED", trustErr: errors.New("damaged trust")}
	view := Execute(context.Background(), func() (Client, error) { return c, nil }, "disconnect", "")
	if c.disconnects != 1 || view.State != "LOCAL" {
		t.Fatalf("blocked disconnect: %+v", view)
	}
}

func TestFaultedPanelBlocksActionsAndExplainsRecovery(t *testing.T) {
	c := &fakeClient{state: "FAULTED", trustErr: errors.New("private trust failure")}
	for _, action := range []string{"refresh", "connect", "disconnect"} {
		view := Execute(context.Background(), func() (Client, error) { return c, nil }, action, "B")
		if view.State != "FAULTED" || !strings.Contains(view.Notice, "차단") || c.connects != 0 || c.disconnects != 0 {
			t.Fatalf("unsafe fault view: %+v", view)
		}
	}
}
