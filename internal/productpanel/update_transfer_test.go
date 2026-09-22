//go:build windows

package productpanel

import (
	"bytes"
	"context"
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
	status   peerupdate.Status
	sent     int
	failSend bool
}

func (c *transferClient) Update(ctx context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	if r.Action == "SEND" {
		c.sent++
		if r.Directory != "native/bundle" {
			panic("renderer selected path")
		}
		if c.failSend {
			return localapi.UpdateResult{}, errors.New("private")
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
	f := &transferClient{settingsClient: c, status: peerupdate.Status{State: "IDLE"}}
	return NewUpdateTransferBridge(context.Background(), func() (UpdateSettingsClient, error) { return f, nil }, "native/bundle"), f, peer
}
func TestUpdateTransferTracksExactJobAndRuntime(t *testing.T) {
	b, c, peer := transferTestBridge()
	if b.TransferStatus(peer).State != "IDLE" || c.sent != 0 {
		t.Fatal("poll sent update")
	}
	if b.Send(peer).State != "QUEUED" || c.sent != 1 {
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
	bundle, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metadata().Revision != os.Getenv("KRASIS_UPDATE_REVISION") || bundle.Metadata().Generation != localapi.AgentGeneration {
		t.Fatal("build provenance mismatch")
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
	err = launchReceivedUpdate(filepath.Join(parent, id), bundle.Metadata().Revision, "A", 32, func(cmd *exec.Cmd) error {
		called = true
		if filepath.Base(cmd.Path) != "krasis-control.exe" {
			t.Fatal("wrong receiver")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("g32 launch preparation: %v", err)
	}
}
