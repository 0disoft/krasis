package productpanel

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
)

type layoutClient struct {
	fakeClient
	request localapi.ConnectionRequest
}

func (c *layoutClient) Connection(_ context.Context, r localapi.ConnectionRequest) (localapi.ConnectionResult, error) {
	c.request = r
	return localapi.ConnectionResult{Status: "COMPLETED", Code: "connection_saved"}, nil
}
func TestLayoutUsesFingerprintAndNeverSuppliesAddress(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, tc := range []struct {
		state, fingerprint, edge string
		want                     bool
	}{
		{"LOCAL", id, "LEFT", true}, {"LOCAL", id, "RIGHT", true}, {"CONNECTED", id, "LEFT", false},
		{"LOCAL", strings.Repeat("b", 64), "LEFT", false}, {"LOCAL", id, "UP", false},
	} {
		c := &layoutClient{fakeClient: fakeClient{state: tc.state, peers: []localapi.TrustedPeer{{Alias: "peer", Fingerprint: id}}}}
		SetLayout(context.Background(), func() (Client, error) { return c, nil }, tc.fingerprint, tc.edge)
		if (c.request.Action == "SET_EDGE") != tc.want || c.request.Address != "" {
			t.Fatal("unexpected layout write")
		}
		if tc.want && c.request.Fingerprint != id {
			t.Fatal("identity was not bound")
		}
	}
}

func TestRearmRequestPreservesOtherSettings(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, distance := range []int32{32, 64, 128, 0, 33} {
		c := &layoutClient{fakeClient: fakeClient{state: "LOCAL", peers: []localapi.TrustedPeer{{Alias: "peer", Fingerprint: id}}}}
		SetRearm(context.Background(), func() (Client, error) { return c, nil }, id, distance)
		want := distance == 32 || distance == 64 || distance == 128
		if (c.request.Action == "SET_REARM") != want || c.request.Address != "" || c.request.Edge != "" {
			t.Fatal("unexpected rearm request")
		}
		if want && (c.request.Fingerprint != id || c.request.RearmDistance != distance) {
			t.Fatal("rearm identity or value lost")
		}
	}
}
