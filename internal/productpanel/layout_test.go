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
