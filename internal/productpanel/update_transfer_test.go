//go:build windows

package productpanel

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/updatebundle"
)

type transferClient struct {
	*settingsClient
	status     peerupdate.Status
	sent       int
	sentDir    string
	failSend   bool
	rejectSend bool
}

func (c *transferClient) Update(ctx context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	if r.Action == "SEND" {
		c.sent++
		c.sentDir = r.Directory
		if c.failSend {
			return localapi.UpdateResult{}, errors.New("private")
		}
		if c.rejectSend {
			return localapi.UpdateResult{Code: "rejected", Status: c.status}, nil
		}
		c.status = peerupdate.Status{Peer: r.Peer, ID: strings.Repeat("d", 32), State: "QUEUED", Generation: 34, Revision: strings.Repeat("c", 40), Total: 100}
		return localapi.UpdateResult{Code: "queued", Status: c.status}, nil
	}
	if r.Action == "STATUS" {
		return localapi.UpdateResult{Code: "ok", Status: c.status}, nil
	}
	return c.settingsClient.Update(ctx, r)
}
func transferTestBridge() (*UpdateSettingsBridge, *transferClient, string) {
	c, peer := newSettingsTestClient()
	c.runtime.Generation = localapi.AgentGeneration
	f := &transferClient{settingsClient: c, status: peerupdate.Status{State: "IDLE"}}
	return NewUpdateTransferBridge(context.Background(), func() (UpdateSettingsClient, error) { return f, nil }, "native/bundle", "native/transition", "native/probe-transition"), f, peer
}
func TestUpdateTransferRefusesMismatchedResidentAgent(t *testing.T) {
	b, c, peer := transferTestBridge()
	c.runtime.Generation--
	if got := b.Send(peer); got.State != "AGENT_MISMATCH" || c.sent != 0 {
		t.Fatalf("stale agent sent update: %+v", got)
	}
}
func TestUpdateTransferTracksExactJobAndRuntime(t *testing.T) {
	b, c, peer := transferTestBridge()
	if b.TransferStatus(peer).State != "IDLE" || c.sent != 0 {
		t.Fatal("poll sent update")
	}
	if b.Send(peer).State != "QUEUED" || c.sent != 1 || c.sentDir != "native/transition" {
		t.Fatal("send failed")
	}
	if b.Send(peer).State != "PEER_BUSY" || c.sent != 1 {
		t.Fatal("duplicate send")
	}
	c.status.State = "DELIVERED"
	if b.TransferStatus(peer).State != "DELIVERED" {
		t.Fatal("delivery misreported")
	}
	c.status.State = "UPDATED"
	if b.TransferStatus(peer).State != "UPDATED" {
		t.Fatal("completion lost")
	}
	c.status.ID = strings.Repeat("e", 32)
	if b.TransferStatus(peer).State != "TRANSFER_UNKNOWN" {
		t.Fatal("other job used")
	}
	c.runtime.Instance = "replacement"
	if b.TransferStatus(peer).State != "AGENT_CHANGED" {
		t.Fatal("new process reused")
	}
}

func TestUpdateTransferChoosesTransitionBundleForOlderReceivers(t *testing.T) {
	for _, tc := range []struct {
		generation int
		state      string
		wantDir    string
	}{
		{60, "ONLINE", "native/transition"},
		{61, "ONLINE", "native/transition"},
		{74, "ONLINE", "native/transition"},
		{75, "ONLINE", "native/probe-transition"},
		{89, "ONLINE", "native/probe-transition"},
		{90, "ONLINE", "native/bundle"},
		{60, "STALE", ""},
	} {
		b, c, peer := transferTestBridge()
		c.info.Generation = tc.generation
		c.info.State = tc.state
		got := b.Send(peer)
		if tc.wantDir == "" {
			if got.State != "PEER_UNREACHABLE" || c.sent != 0 {
				t.Fatalf("stale peer sent update: %+v", got)
			}
		} else if got.State != "QUEUED" || c.sentDir != tc.wantDir {
			t.Fatalf("wrong update bundle: %+v, %q", got, c.sentDir)
		}
	}
	b, c, peer := transferTestBridge()
	b.transitionDirectory = ""
	c.info.Generation = 60
	if got := b.Send(peer); got.State != "TRANSITION_REQUIRED" || c.sent != 0 {
		t.Fatalf("missing legacy bundle sent update: %+v", got)
	}
	b, c, peer = transferTestBridge()
	b.probeTransitionDirectory = ""
	c.info.Generation = 89
	if got := b.Send(peer); got.State != "TRANSITION_REQUIRED" || c.sent != 0 {
		t.Fatalf("missing g90 bundle sent update: %+v", got)
	}
}
func TestUpdateTransferSkipsKnownCurrentOrNewerPeer(t *testing.T) {
	for _, test := range []struct {
		generation int
		revision   string
		want       string
	}{
		{localapi.AgentGeneration, strings.Repeat("c", 40), "SAME_VERSION"},
		{localapi.AgentGeneration, strings.Repeat("d", 40), "GENERATION_CONFLICT"},
		{localapi.AgentGeneration + 1, strings.Repeat("d", 40), "PEER_NEWER"},
	} {
		b, c, peer := transferTestBridge()
		c.info.Generation, c.info.Revision = test.generation, test.revision
		if got := b.Send(peer); got.State != test.want || c.sent != 0 {
			t.Fatalf("sent to known current/newer peer: %+v", got)
		}
	}
}

func TestUnknownTransferResolvesOnlyFromLiveExactPeerBuild(t *testing.T) {
	b, c, peer := transferTestBridge()
	c.failSend = true
	if b.Send(peer).State != "TRANSFER_UNKNOWN" {
		t.Fatal("uncertain send became definite")
	}
	c.info.Generation = localapi.AgentGeneration
	c.info.Revision = c.runtime.Build
	c.info.State = "STALE"
	if b.TransferStatus(peer).State != "TRANSFER_UNKNOWN" {
		t.Fatal("stale observation resolved transfer")
	}
	c.info.State = "ONLINE"
	if b.TransferStatus(peer).State != "SAME_VERSION" || c.sent != 1 {
		t.Fatal("exact live build did not prevent resend")
	}
}

func TestUpdateTransferRejectionDoesNotAdoptPreviousJob(t *testing.T) {
	b, c, peer := transferTestBridge()
	c.rejectSend = true
	c.status = peerupdate.Status{Peer: strings.Repeat("b", 64), ID: strings.Repeat("e", 32), State: "UPDATED", Generation: 999, Revision: strings.Repeat("c", 40)}
	v := b.Send(peer)
	if v.State != "REJECTED" || v.ID != "" || v.Generation != 0 || v.Build != "" {
		t.Fatal(v)
	}
	if b.TransferStatus(peer).State != "REJECTED" {
		t.Fatal("foreign result reused")
	}
}
func TestUpdateTransferRejectsRevokedAndDoesNotRetryUnknown(t *testing.T) {
	b, c, peer := transferTestBridge()
	c.peers[0].Revoked = true
	if b.Send(peer).State != "STALE_PEER" || c.sent != 0 {
		t.Fatal("revoked peer sent")
	}
	c.peers[0].Revoked = false
	c.failSend = true
	if b.Send(peer).State != "TRANSFER_UNKNOWN" {
		t.Fatal("timeout claimed failure")
	}
	for range 3 {
		if b.TransferStatus(peer).State != "TRANSFER_UNKNOWN" {
			t.Fatal("unknown lost")
		}
	}
	if c.sent != 1 {
		t.Fatal("automatic resend")
	}
}
func TestUpdateTransferProjectionRejectsPrivateAndInvalidData(t *testing.T) {
	v := projectTransfer("peer", peerupdate.Status{ID: "private", Bytes: 200, Total: 100, Generation: -1, Revision: "private"}, "private/path")
	if v.State != "TRANSFER_UNKNOWN" || v.ID != "" || v.Bytes != 0 || v.Total != 0 || v.Build != "" {
		t.Fatal(v)
	}
}
func TestWorkspaceUpdatePackage(t *testing.T) {
	directory := os.Getenv("KRASIS_WORKSPACE_UPDATE")
	if directory == "" {
		t.Skip("packaging check only")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data, err := updatebundle.Pack(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wire update archive: %d bytes", len(data))
	bundle, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metadata().Revision != os.Getenv("KRASIS_UPDATE_REVISION") || bundle.Metadata().Generation != localapi.AgentGeneration {
		t.Fatal("build provenance mismatch")
	}
	controlInfo, err := buildinfo.ReadFile(filepath.Join(directory, "krasis.exe"))
	if err != nil {
		t.Fatal(err)
	}
	desktop := false
	for _, setting := range controlInfo.Settings {
		if setting.Key == "-tags" && strings.Contains(setting.Value, "desktop") && strings.Contains(setting.Value, "production") {
			desktop = true
		}
	}
	if !desktop {
		t.Fatal("update still contains legacy UI instead of Workspace")
	}
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	parent := filepath.Join(base, "Krasis", "updates")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := peercredentials.ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	staged, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Close()
	id := strings.Repeat("a", 32)
	if err := bundle.Stage(staged, id); err != nil {
		t.Fatal(err)
	}
	called := false
	err = launchReceivedUpdate(filepath.Join(parent, id), bundle.Metadata().Revision, "A", 34, func(cmd *exec.Cmd) error {
		called = true
		if filepath.Base(cmd.Path) != "krasis.exe" {
			t.Fatal("wrong receiver")
		}
		if cmd.SysProcAttr == nil || cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags != 0x08000000 {
			t.Fatal("updated GUI must remain visible without creating a console")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("g34 launch preparation: %v", err)
	}
}
