package productpanel

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
)

type revokeClient struct {
	fakeClient
	revoked string
}

func (c *revokeClient) RevokeTrust(_ context.Context, fingerprint string) (localapi.OperationResult, error) {
	c.revoked = fingerprint
	c.peers = nil
	return localapi.OperationResult{Status: "COMPLETED", Code: "trust_revoked"}, nil
}

func TestRevokeUsesConfirmedFingerprintAndRejectsStaleSelection(t *testing.T) {
	old, newID := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		state, id string
		want      bool
	}{{"LOCAL", old, true}, {"CONNECTED", old, false}, {"LOCAL", newID, false}} {
		c := &revokeClient{fakeClient: fakeClient{state: tc.state, peers: []localapi.TrustedPeer{{Alias: "peer", Fingerprint: tc.id}}}}
		view := Revoke(context.Background(), func() (Client, error) { return c, nil }, old)
		if (c.revoked == old) != tc.want {
			t.Fatal("revoked an active or changed peer")
		}
		if tc.want && len(view.Peers) != 0 {
			t.Fatal("revoked peer remains selectable")
		}
	}
}
