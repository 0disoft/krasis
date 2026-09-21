package productpanel

import (
	"context"
	"encoding/json"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/truststore"
	"strings"
	"testing"
)

type layoutClient struct {
	fakeClient
	request localapi.ConnectionRequest
	profile truststore.ConnectionProfile
}

func (c *layoutClient) Connection(_ context.Context, r localapi.ConnectionRequest) (localapi.ConnectionResult, error) {
	c.request = r
	if r.Action == "SHOW" {
		if c.profile.Edge == "" {
			return localapi.ConnectionResult{Status: "REJECTED", Code: "connection_not_configured"}, nil
		}
		p := c.profile
		return localapi.ConnectionResult{Status: "COMPLETED", Code: "connection_loaded", Profile: &p}, nil
	}
	if c.profile.Edge != "" {
		p := c.profile
		if r.Edge != "" {
			p.Edge = r.Edge
		}
		if r.RearmDistance != 0 {
			p.RearmDistance = r.RearmDistance
		}
		return localapi.ConnectionResult{Status: "COMPLETED", Code: "connection_saved", Profile: &p}, nil
	}
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

func TestNativeLayoutOmitsAddressAndRequiresLocalWrites(t *testing.T) {
	id := strings.Repeat("a", 64)
	c := &layoutClient{fakeClient: fakeClient{state: "LOCAL", peers: []localapi.TrustedPeer{{Alias: "peer", Fingerprint: id}}}}
	c.profile = truststore.ConnectionProfile{SchemaVersion: truststore.ConnectionSchemaVersion, Fingerprint: id, ConnectionSettings: truststore.ConnectionSettings{Address: "192.168.1.9:45872", Edge: "RIGHT", RearmDistance: 64}}
	status := LayoutStatus(context.Background(), func() (Client, error) { return c, nil }, id)
	raw, _ := json.Marshal(status)
	if status.Edge != "RIGHT" || status.Rearm != 64 || status.Code != "ok" || strings.Contains(string(raw), "192.168") {
		t.Fatal(string(raw))
	}
	c.fakeClient.state = "CONNECTED"
	if LayoutSet(context.Background(), func() (Client, error) { return c, nil }, id, "LEFT").Code != "not_local" {
		t.Fatal("connected layout write accepted")
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
