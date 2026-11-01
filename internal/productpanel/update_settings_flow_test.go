package productpanel

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

type settingsFlowClient struct {
	*settingsClient
	calls         []string
	trustErr      error
	reply         *localapi.UpdateResult
	updateErr     error
	cancelRuntime context.CancelFunc
}

func (c *settingsFlowClient) Trust(ctx context.Context) (localapi.Trust, error) {
	c.calls = append(c.calls, "TRUST")
	result, err := c.settingsClient.Trust(ctx)
	if c.trustErr != nil {
		return result, c.trustErr
	}
	return result, err
}

func (c *settingsFlowClient) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	c.calls = append(c.calls, "RUNTIME")
	result, err := c.settingsClient.Runtime(ctx)
	if c.cancelRuntime != nil {
		c.cancelRuntime()
	}
	return result, err
}

func (c *settingsFlowClient) Update(ctx context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	c.calls = append(c.calls, r.Action)
	if c.updateErr != nil {
		return localapi.UpdateResult{}, c.updateErr
	}
	if c.reply != nil && r.Action != "INSPECT" {
		return *c.reply, nil
	}
	return c.settingsClient.Update(ctx, r)
}

func TestUpdateSettingsFlowPreservesCallsAndResponses(t *testing.T) {
	for _, tc := range []struct {
		name, action, want, permission, calls string
		setup                                 func(*settingsFlowClient)
	}{
		{"allow", "ALLOW", "permission_saved", "ALLOWED", "TRUST RUNTIME ALLOW RUNTIME", nil},
		{"deny", "DENY", "permission_saved", "DENIED", "TRUST RUNTIME DENY RUNTIME", nil},
		{"status", "STATUS", "ok", "DENIED", "TRUST RUNTIME STATUS INSPECT", nil},
		{"revoked", "ALLOW", "stale_peer", "UNKNOWN", "TRUST", func(c *settingsFlowClient) { c.peers[0].Revoked = true }},
		{"trust failure", "ALLOW", "unavailable", "UNKNOWN", "TRUST", func(c *settingsFlowClient) { c.trustErr = errors.New("lookup failed") }},
		{"lost permission reply", "ALLOW", "outcome_unknown", "UNKNOWN", "TRUST RUNTIME ALLOW", func(c *settingsFlowClient) { c.updateErr = errors.New("reply lost") }},
		{"lost status reply", "STATUS", "unavailable", "UNKNOWN", "TRUST RUNTIME STATUS", func(c *settingsFlowClient) { c.updateErr = errors.New("reply lost") }},
		{"agent replaced", "ALLOW", "agent_changed", "UNKNOWN", "TRUST RUNTIME ALLOW RUNTIME", func(c *settingsFlowClient) { c.changed = true }},
		{"runtime unavailable", "ALLOW", "permission_saved", "ALLOWED", "TRUST RUNTIME ALLOW", func(c *settingsFlowClient) { c.missingRuntime = true }},
		{"instance empty", "ALLOW", "permission_saved", "ALLOWED", "TRUST RUNTIME ALLOW", func(c *settingsFlowClient) { c.runtime.Instance = "" }},
		{"legacy status", "STATUS", "peer_status_unavailable", "DENIED", "TRUST RUNTIME STATUS", func(c *settingsFlowClient) { c.runtime.Generation = 32 }},
		{"unknown runtime still inspects", "STATUS", "ok", "DENIED", "TRUST RUNTIME STATUS INSPECT", func(c *settingsFlowClient) { c.missingRuntime = true }},
		{"allow acknowledgment mismatch", "ALLOW", "outcome_unknown", "UNKNOWN", "TRUST RUNTIME ALLOW RUNTIME", func(c *settingsFlowClient) {
			c.reply = &localapi.UpdateResult{Code: "permission_saved"}
		}},
		{"deny acknowledgment mismatch", "DENY", "outcome_unknown", "UNKNOWN", "TRUST RUNTIME DENY RUNTIME", func(c *settingsFlowClient) {
			c.reply = &localapi.UpdateResult{Code: "permission_saved", Status: peerupdate.Status{Allowed: []string{c.peers[0].Fingerprint}}}
		}},
		{"replacement before acknowledgment mismatch", "ALLOW", "agent_changed", "UNKNOWN", "TRUST RUNTIME ALLOW RUNTIME", func(c *settingsFlowClient) {
			c.changed = true
			c.reply = &localapi.UpdateResult{Code: "permission_saved"}
		}},
		{"unsupported", "ALLOW", "unsupported", "UNKNOWN", "TRUST RUNTIME ALLOW", func(c *settingsFlowClient) {
			c.reply = &localapi.UpdateResult{Code: "unsupported"}
		}},
		{"rejected", "ALLOW", "rejected", "UNKNOWN", "TRUST RUNTIME ALLOW", func(c *settingsFlowClient) {
			c.reply = &localapi.UpdateResult{Code: "rejected"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, peer := newSettingsTestClient()
			client := &settingsFlowClient{settingsClient: base}
			if tc.setup != nil {
				tc.setup(client)
			}
			b := NewUpdateSettingsBridge(context.Background(), func() (UpdateSettingsClient, error) { return client, nil })
			got := b.execute(peer, tc.action)
			if got.Code != tc.want || got.Permission != tc.permission || strings.Join(client.calls, " ") != tc.calls {
				t.Fatalf("code=%s permission=%s calls=%v", got.Code, got.Permission, client.calls)
			}
			if got.Code == "agent_changed" && got.LocalGeneration != 0 {
				t.Fatal("agent replacement retained stale version evidence")
			}
		})
	}
}

func TestUpdateSettingsCancellationAfterRuntimePreventsPermissionWrite(t *testing.T) {
	base, peer := newSettingsTestClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &settingsFlowClient{settingsClient: base, cancelRuntime: cancel}
	b := NewUpdateSettingsBridge(ctx, func() (UpdateSettingsClient, error) { return client, nil })
	got := b.Permit(peer, true)
	if got.Code != "cancelled" || got.LocalGeneration != base.runtime.Generation || strings.Join(client.calls, " ") != "TRUST RUNTIME" {
		t.Fatalf("late cancellation changed dispatch or version evidence: %+v calls=%v", got, client.calls)
	}
}

func TestUpdateSettingsProjectionsLeaveInputsUnchanged(t *testing.T) {
	peer := strings.Repeat("a", 64)
	input := updateSettingsFailure(peer, "unavailable")
	before := input
	local := withLocalUpdateVersion(input, localapi.RuntimeInfo{Generation: 33, Build: strings.Repeat("C", 40)})
	if input != before || local.LocalGeneration != 33 || local.LocalBuild != strings.Repeat("c", 40) {
		t.Fatal("local version projection changed its input or lost normalized build")
	}
	info := peerupdate.PeerInspection{Peer: peer, State: "ONLINE", Generation: 33, Revision: strings.Repeat("c", 40), Allowed: true}
	infoBefore := info
	inspected := projectUpdateInspection(local, info)
	if local.PeerState != "UNKNOWN" || info != infoBefore || inspected.Comparison != "SAME_BUILD" || inspected.RemotePermission != "ALLOWED" {
		t.Fatal("inspection projection changed its inputs or lost peer evidence")
	}
	backing := []string{peer, "caller-owned-sentinel"}
	backingBefore := slices.Clone(backing)
	result := localapi.UpdateResult{Code: "permission_saved", Status: peerupdate.Status{Allowed: backing[:1]}}
	reply, accepted := projectUpdateSettingsReply(input, "ALLOW", result, nil)
	ack := projectUpdatePermissionAck(reply, "DENY")
	if !accepted || reply.Code != "permission_saved" || reply.Permission != "ALLOWED" || ack.Code != "outcome_unknown" || ack.Permission != "UNKNOWN" {
		t.Fatal("acknowledgment projection lost uncertainty or mutated its input")
	}
	if input != before || !slices.Equal(backing, backingBefore) {
		t.Fatal("reply projection changed the caller's snapshot or permission array")
	}
}

func TestUpdateSettingsProjectionGenerationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		generation, local int
		remote            string
	}{
		{0, 0, "UNKNOWN"}, {1, 1, "UNKNOWN"}, {20, 20, "UNKNOWN"},
		{21, 21, "ONLINE"}, {1000000, 1000000, "ONLINE"}, {1000001, 0, "UNKNOWN"},
	} {
		input := updateSettingsFailure("peer", "ok")
		local := withLocalUpdateVersion(input, localapi.RuntimeInfo{Generation: tc.generation})
		remote := projectUpdateInspection(input, peerupdate.PeerInspection{State: "ONLINE", Generation: tc.generation})
		if local.LocalGeneration != tc.local || remote.PeerState != tc.remote || input.LocalGeneration != 0 || input.PeerState != "UNKNOWN" {
			t.Fatalf("generation=%d local=%+v remote=%+v input=%+v", tc.generation, local, remote, input)
		}
	}
}
