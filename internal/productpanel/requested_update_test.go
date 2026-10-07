package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type requestedUpdateClient struct{ releaseClient }

func (c *requestedUpdateClient) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	return localapi.RuntimeInfo{Instance: strings.Repeat("a", 32), Protocol: localapi.AgentProtocol, Generation: localapi.AgentGeneration}, nil
}

func (c *requestedUpdateClient) Status(ctx context.Context) (localapi.Status, error) {
	value, err := c.fakeClient.Status(ctx)
	if value.State == "CONNECTED" || value.State == "RECONNECTING" {
		value.PeerAlias = "B"
	}
	return value, err
}

func TestRequestedUpdateSafelyDisconnectsAndPassesTheSavedIdentity(t *testing.T) {
	for _, session := range []string{"CONNECTED", "RECONNECTING", "LOCAL"} {
		t.Run(session, func(t *testing.T) {
			fingerprint := strings.Repeat("a", 64)
			client := &requestedUpdateClient{releaseClient: releaseClient{fakeClient: &fakeClient{state: session, peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fingerprint}}}}}
			b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
			launches := 0
			launch := func(alias, fp string) error {
				launches++
				if client.state != "LOCAL" || !client.claimed {
					t.Fatal("launch before disconnected input/claim")
				}
				if session != "LOCAL" && (alias != "B" || fp != fingerprint || client.disconnects != 1) {
					t.Fatal(alias, fp, client.disconnects)
				}
				if session == "LOCAL" && (alias != "" || fp != "" || client.disconnects != 0) {
					t.Fatal("created a session from idle")
				}
				return nil
			}
			if applied, err := b.ApplyRequestedUpdate(localapi.AgentGeneration+1, strings.Repeat("b", 40), launch); !applied || err != nil {
				t.Fatal(applied, err)
			}
			b.ApplyRequestedUpdate(localapi.AgentGeneration+1, strings.Repeat("b", 40), launch)
			if launches != 1 || !b.updateHandedOff || !b.stopped.Load() {
				t.Fatal(launches, b)
			}
		})
	}
}

func TestRequestedUpdateRestoresOnlyTheSamePeerOnLaunchFailure(t *testing.T) {
	for _, changed := range []bool{false, true} {
		client := &requestedUpdateClient{releaseClient: releaseClient{fakeClient: &fakeClient{state: "CONNECTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("a", 64)}}}}}
		b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
		launchErr := errors.New("launch failed")
		applied, err := b.ApplyRequestedUpdate(localapi.AgentGeneration+1, strings.Repeat("b", 40), func(string, string) error {
			if changed {
				client.peers[0].Fingerprint = strings.Repeat("b", 64)
			}
			return launchErr
		})
		wantConnects := 1
		if changed {
			wantConnects = 0
		}
		if applied || !errors.Is(err, launchErr) || client.claimed || client.releases != 1 || client.connects != wantConnects || b.updateHandedOff {
			t.Fatal(applied, err, client, b.Refresh())
		}
	}
}

func TestRequestedUpdateDoesNotInterruptPairingOrInvalidTargets(t *testing.T) {
	for _, tc := range []struct {
		state, revision string
		generation      int
	}{
		{"PAIRING", strings.Repeat("a", 40), localapi.AgentGeneration + 1},
		{"CONNECTED", "bad", localapi.AgentGeneration + 1},
		{"CONNECTED", strings.Repeat("a", 40), localapi.AgentGeneration},
	} {
		client := &requestedUpdateClient{releaseClient: releaseClient{fakeClient: &fakeClient{state: tc.state}}}
		b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
		applied, err := b.ApplyRequestedUpdate(tc.generation, tc.revision, func(string, string) error { t.Fatal("unexpected launch"); return nil })
		if applied || err == nil || client.disconnects != 0 || client.claimed {
			t.Fatal(applied, err, client)
		}
	}
}

func TestRequestedUpdateArgumentsBindReconnectionToFingerprint(t *testing.T) {
	revision, fingerprint := strings.Repeat("a", 40), strings.Repeat("b", 64)
	args := []string{"--updated-peer", "B", "--expected-build", revision, "--wait-pid", "123", "--peer-fingerprint", fingerprint}
	value, err := ParseWorkspaceUpdateLaunch(args)
	if err != nil || value.Fingerprint != fingerprint || value.Alias != "B" {
		t.Fatal(value, err)
	}
	args[7] = "bad"
	if _, err = ParseWorkspaceUpdateLaunch(args); err == nil {
		t.Fatal("invalid fingerprint accepted")
	}
	client := &receiverClient{settingsClient: &settingsClient{runtime: localapi.RuntimeInfo{Build: revision}, peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("c", 64)}}}}
	if err = ResumeUpdatedWorkspace(context.Background(), client, value); err == nil {
		t.Fatal("changed identity resumed")
	}
}
