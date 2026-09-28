package productpanel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

type settingsClient struct {
	peers                                 []localapi.TrustedPeer
	runtime                               localapi.RuntimeInfo
	info                                  *peerupdate.PeerInspection
	allowed                               []string
	actions                               []localapi.UpdateRequest
	legacy, fail, changed, missingRuntime bool
	runtimeCalls                          int
}

func (c *settingsClient) Trust(context.Context) (localapi.Trust, error) {
	return localapi.Trust{Peers: c.peers}, nil
}
func (c *settingsClient) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	c.runtimeCalls++
	v := c.runtime
	if c.changed && c.runtimeCalls > 1 {
		v.Instance = "replacement"
	}
	if c.missingRuntime {
		return v, errors.New("secret")
	}
	return v, nil
}
func (c *settingsClient) Update(_ context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	c.actions = append(c.actions, r)
	v := localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Status: peerupdate.Status{Allowed: c.allowed, Peer: "other", PeerRevision: "private", ReceivedDiagnostic: "private/path"}}
	switch r.Action {
	case "INSPECT":
		v.Code = "peer_status"
		v.PeerInfo = c.info
		if c.legacy {
			v.Code = "rejected"
			v.PeerInfo = nil
		}
	case "ALLOW":
		v.Code = "permission_saved"
		v.Status.Allowed = []string{r.Peer}
		if c.fail {
			return v, errors.New("private token")
		}
	case "DENY":
		v.Code = "permission_saved"
		v.Status.Allowed = []string{}
	}
	return v, nil
}
func newSettingsTestClient() (*settingsClient, string) {
	peer := strings.Repeat("a", 64)
	return &settingsClient{peers: []localapi.TrustedPeer{{Fingerprint: peer}}, runtime: localapi.RuntimeInfo{Generation: 33, Build: strings.Repeat("c", 40), Instance: "current"}, info: &peerupdate.PeerInspection{Peer: peer, State: "ONLINE", Generation: 33, Revision: strings.Repeat("c", 40), Allowed: false}}, peer
}
func settingsBridge(c *settingsClient) *UpdateSettingsBridge {
	return NewUpdateSettingsBridge(context.Background(), func() (UpdateSettingsClient, error) { return c, nil })
}

func TestUpdateSettingsSeparatesPermissionDirectionsAndRedactsMetadata(t *testing.T) {
	c, peer := newSettingsTestClient()
	c.allowed = []string{peer}
	v := settingsBridge(c).Status(peer)
	raw, _ := json.Marshal(v)
	if v.Code != "ok" || v.AppGeneration != localapi.AgentGeneration || v.Permission != "ALLOWED" || v.RemotePermission != "DENIED" || v.Comparison != "SAME_BUILD" || strings.Contains(string(raw), "private") {
		t.Fatalf("projection: %s", raw)
	}
	for _, r := range c.actions {
		if r.Action != "STATUS" && r.Action != "INSPECT" || r.Directory != "" || r.ID != "" {
			t.Fatal("status had side effects")
		}
	}
}
func TestUpdateSettingsOldAgentAndStalePeerNeverReuseGlobalVersion(t *testing.T) {
	for _, mode := range []string{"legacy", "stale", "other"} {
		c, peer := newSettingsTestClient()
		switch mode {
		case "legacy":
			c.legacy = true
			c.runtime.Generation = 32
		case "stale":
			c.info.State = "STALE"
		case "other":
			c.info.Peer = strings.Repeat("b", 64)
		}
		v := settingsBridge(c).Status(peer)
		if v.Permission != "DENIED" || v.PeerBuild != "" || v.PeerGeneration != 0 || v.Comparison != "UNKNOWN" || v.RemotePermission != "UNKNOWN" {
			t.Fatalf("stale/cross-peer metadata: %+v", v)
		}
		if mode == "legacy" && v.Code != "peer_status_unavailable" {
			t.Fatal("old agent support guessed")
		}
	}
}
func TestUpdateSettingsPermissionRechecksTrustAndAcknowledgment(t *testing.T) {
	for _, mode := range []string{"allow", "deny", "revoked", "fail", "changed", "no-runtime"} {
		c, peer := newSettingsTestClient()
		c.peers[0].Revoked = mode == "revoked"
		c.fail = mode == "fail"
		c.changed = mode == "changed"
		c.missingRuntime = mode == "no-runtime"
		v := settingsBridge(c).Permit(peer, mode != "deny")
		switch mode {
		case "revoked":
			if len(c.actions) != 0 || v.Code != "stale_peer" {
				t.Fatal("revoked permission mutated")
			}
		case "fail":
			if v.Code != "outcome_unknown" || v.Permission != "UNKNOWN" {
				t.Fatal("failed mutation appeared saved")
			}
		case "changed":
			if v.Code != "agent_changed" || v.Permission != "UNKNOWN" {
				t.Fatal("agent replacement appeared confirmed")
			}
		default:
			if v.Code != "permission_saved" || (v.Permission == "ALLOWED") != (mode != "deny") {
				t.Fatalf("acknowledgment: %+v", v)
			}
		}
		for _, r := range c.actions {
			if (r.Action != "ALLOW" && r.Action != "DENY") || r.Peer != peer || r.Directory != "" || r.ID != "" {
				t.Fatal("permission escaped bounded action")
			}
		}
	}
}
func TestUpdateSettingsComparisonsDoNotInventNewestBuild(t *testing.T) {
	for _, tc := range []struct {
		gen         int
		build, want string
	}{{32, strings.Repeat("d", 40), "LOCAL_NEWER"}, {34, strings.Repeat("d", 40), "PEER_NEWER"}, {33, strings.Repeat("d", 40), "DIFFERENT_BUILD"}, {33, "secret/path", "UNKNOWN"}, {33, "", "UNKNOWN"}} {
		c, peer := newSettingsTestClient()
		c.info.Generation = tc.gen
		c.info.Revision = tc.build
		v := settingsBridge(c).Status(peer)
		if v.Comparison != tc.want || strings.Contains(v.PeerBuild, "secret") {
			t.Fatalf("comparison: %+v", v)
		}
	}
}
func TestUpdateSettingsRejectsMalformedBusyAndCancelledRequests(t *testing.T) {
	c, peer := newSettingsTestClient()
	b := settingsBridge(c)
	if b.Permit("../private", true).Code != "invalid_peer" || len(c.actions) != 0 {
		t.Fatal("malformed action dispatched")
	}
	b.mu.Lock()
	v := b.Permit(peer, true)
	b.mu.Unlock()
	if v.Code != "busy" || len(c.actions) != 0 {
		t.Fatal("busy action queued")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b = NewUpdateSettingsBridge(ctx, func() (UpdateSettingsClient, error) { t.Fatal("cancelled client opened"); return c, nil })
	if b.Status(peer).Code != "cancelled" {
		t.Fatal("cancelled request ran")
	}
}
