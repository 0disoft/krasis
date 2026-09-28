package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestWorkspaceUpdateArguments(t *testing.T) {
	rev := strings.Repeat("a", 40)
	for _, args := range [][]string{{"--updated-peer", "A", "--expected-build", rev}, {"--updated-peer", "A", "--expected-build", rev, "--wait-pid", "123"}} {
		got, err := ParseWorkspaceUpdateLaunch(args)
		if err != nil || got.Alias != "A" || got.Revision != rev {
			t.Fatal(got, err)
		}
	}
	for _, args := range [][]string{nil, {"--updated-peer", "A"}, {"--updated-peer", "", "--expected-build", rev}, {"--updated-peer", "A", "--expected-build", "private/path"}, {"--updated-peer", "A", "--expected-build", rev, "--wait-pid", "0"}, {"--updated-peer", "A", "--expected-build", rev, "--wait-pid", "-1"}} {
		if _, err := ParseWorkspaceUpdateLaunch(args); err == nil {
			t.Fatal("accepted", args)
		}
	}
}

type receiverClient struct {
	*settingsClient
	status                     peerupdate.Status
	applied, failed, connected int
	mismatch, transport        bool
}

func (c *receiverClient) Update(_ context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	out := localapi.UpdateResult{Code: "ok", Status: c.status, Alias: "B", Directory: "native/staged"}
	switch r.Action {
	case "APPLY":
		c.applied++
		out.Code = "apply_ready"
		if r.ID != c.status.ID || r.Peer != c.status.Peer {
			panic("wrong update claim")
		}
		if c.mismatch {
			out.Status.ID = strings.Repeat("e", 32)
		}
		if c.transport {
			return out, errors.New("private")
		}
	case "APPLY_FAILED":
		c.failed++
		out.Code = "apply_failed"
	}
	return out, nil
}
func (c *receiverClient) Connect(context.Context, string) (localapi.OperationResult, error) {
	c.connected++
	return localapi.OperationResult{Status: "COMPLETED"}, nil
}
func newReceiverClient() *receiverClient {
	c, peer := newSettingsTestClient()
	c.peers[0].Alias = "B"
	return &receiverClient{settingsClient: c, status: peerupdate.Status{State: "READY", Peer: peer, ID: strings.Repeat("d", 32), Allowed: []string{peer}, Revision: strings.Repeat("c", 40), Generation: 35}}
}
func TestWorkspaceReceiverClaimsOnceAndReportsLaunchFailure(t *testing.T) {
	for _, mode := range []string{"ok", "failed", "mismatch", "transport", "changed", "denied", "revoked", "staged"} {
		t.Run(mode, func(t *testing.T) {
			c := newReceiverClient()
			launches := 0
			switch mode {
			case "mismatch":
				c.mismatch = true
			case "transport":
				c.transport = true
			case "changed":
				c.changed = true
			case "denied":
				c.status.Allowed = nil
			case "revoked":
				c.peers[0].Revoked = true
			case "staged":
				c.status.State = "STAGED"
			}
			b := NewWorkspaceUpdateReceiver(context.Background(), func() (UpdateSettingsClient, error) { return c, nil }, func(dir, rev, alias string) error {
				launches++
				if dir != "native/staged" || rev != c.status.Revision || alias != "B" {
					t.Fatal("launch mismatch")
				}
				if mode == "failed" {
					return errors.New("private")
				}
				return nil
			})
			if got := b.Poll(); got != (mode == "ok") {
				t.Fatal("apply result", got)
			}
			if b.Poll() {
				t.Fatal("repeated handoff")
			}
			switch mode {
			case "ok":
				if c.applied != 1 || launches != 1 || c.failed != 0 {
					t.Fatal(c, launches)
				}
			case "denied", "revoked", "staged":
				if c.applied != 0 || launches != 0 {
					t.Fatal("unauthorized apply")
				}
			default:
				if c.applied != 1 || c.failed != 1 {
					t.Fatal("failure not acknowledged", c.applied, c.failed)
				}
			}
		})
	}
}
func TestUpdatedWorkspaceReconnectRequiresExactBuildAndTrust(t *testing.T) {
	for _, mode := range []string{"ok", "build", "revoked"} {
		c := newReceiverClient()
		v := WorkspaceUpdateLaunch{Alias: "B", Revision: c.runtime.Build}
		if mode == "build" {
			v.Revision = strings.Repeat("e", 40)
		}
		if mode == "revoked" {
			c.peers[0].Revoked = true
		}
		err := ResumeUpdatedWorkspace(context.Background(), c, v)
		if (err == nil) != (mode == "ok") || (c.connected == 1) != (mode == "ok") {
			t.Fatal(mode, err, c.connected)
		}
	}
}

func TestFailedUpdatedWorkspaceReleasesOnlyExactClaim(t *testing.T) {
	for _, mode := range []string{"exact", "wrong build", "wrong generation", "wrong alias", "revoked", "not applying"} {
		t.Run(mode, func(t *testing.T) {
			c := newReceiverClient()
			c.status.State = "APPLYING"
			c.status.Generation = localapi.AgentGeneration
			v := WorkspaceUpdateLaunch{Alias: "B", Revision: c.status.Revision}
			switch mode {
			case "wrong build":
				v.Revision = strings.Repeat("e", 40)
			case "wrong generation":
				c.status.Generation--
			case "wrong alias":
				v.Alias = "other"
			case "revoked":
				c.peers[0].Revoked = true
			case "not applying":
				c.status.State = "READY"
			}
			got := ReportFailedUpdatedWorkspace(context.Background(), c, v)
			if got != (mode == "exact") || (c.failed == 1) != (mode == "exact") {
				t.Fatalf("incorrect failed-apply report: sent=%d result=%t", c.failed, got)
			}
		})
	}
}

func TestWorkspaceUpdateHandoffSerializesRecovery(t *testing.T) {
	c := newReceiverClient()
	b := NewDesktopLifecycle(context.Background(), nil, nil, nil)
	receiver := NewWorkspaceUpdateReceiver(context.Background(), func() (UpdateSettingsClient, error) { return c, nil }, func(string, string, string) error {
		if b.gate.TryLock() {
			b.gate.Unlock()
			t.Fatal("unserialized update")
		}
		return nil
	})
	if !b.ApplyReceivedUpdate(receiver) || !b.stopped.Load() {
		t.Fatal("recovery left running")
	}
}
