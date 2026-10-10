//go:build windows

package loginagent

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
	"golang.org/x/sys/windows"
)

func liveGrantFixture(t *testing.T) (Grant, string, *truststore.Store, *productidentity.Store) {
	t.Helper()
	now := time.Now().UTC()
	base := t.TempDir()
	paths, identity, _, err := productidentity.BootstrapWindows(base, now)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := productidentity.New(paths.Identity)
	local, err := store.OpenTransport(transport.AuthenticatedPeerConnector, now)
	if err != nil {
		t.Fatal(err)
	}
	localPeer, err := transport.AuthenticatedPeerFromCertificate(local.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	remotePaths, _, _, err := productidentity.BootstrapWindows(t.TempDir(), now)
	if err != nil {
		t.Fatal(err)
	}
	remoteStore, _ := productidentity.New(remotePaths.Identity)
	remote, err := remoteStore.OpenTransport(transport.AuthenticatedPeerListener, now)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(remote.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	trust, _ := truststore.New(paths.Trust)
	if _, err := trust.ImportWithCertificate(peer, remote.Certificate[0], "B", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := trust.SetConnection("B", truststore.ConnectionSettings{Address: "127.0.0.1:24800", Edge: "RIGHT"}); err != nil {
		t.Fatal(err)
	}
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: true, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: peer.CertificateSHA256}); err != nil {
		t.Fatal(err)
	}
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	g := testGrant(now)
	g.Owner = user.User.Sid.String()
	g.Peer = peer.CertificateSHA256
	g.Identity = identity.Public().Fingerprint
	g.LocalTransport = localPeer.CertificateSHA256
	return g, base, trust, store
}

func TestLiveGrantHonorsManualStopStartupAndRevocation(t *testing.T) {
	g, base, trust, _ := liveGrantFixture(t)
	now := time.Now().UTC()
	live, err := readGrantConnectionAt(g, base, now)
	if err != nil || !live.Peer.AllowInboundControl {
		t.Fatal("valid live grant", err)
	}
	g.AllowInbound = false
	live, err = readGrantConnectionAt(g, base, now)
	if err != nil || live.Peer.AllowInboundControl {
		t.Fatal("profile escalated inbound grant", err)
	}
	if err := appsettings.SaveResume(base, appsettings.Resume{ManualStop: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := readGrantConnectionAt(g, base, now); !errors.Is(err, ErrSuspended) {
		t.Fatal("manual stop ignored", err)
	}
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: g.Peer}); err != nil {
		t.Fatal(err)
	}
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema}); err != nil {
		t.Fatal(err)
	}
	if _, err := readGrantConnectionAt(g, base, now); !errors.Is(err, ErrSuspended) {
		t.Fatal("startup disable ignored", err)
	}
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := trust.Revoke(g.Peer, now); err != nil {
		t.Fatal(err)
	}
	if _, err := readGrantConnectionAt(g, base, now); !errors.Is(err, ErrGrant) {
		t.Fatal("revocation ignored", err)
	}
}

func TestLiveGrantRefusesCredentialRotationAndPeerChanges(t *testing.T) {
	g, base, _, store := liveGrantFixture(t)
	now := time.Now().UTC()
	for _, change := range []func(*Grant){
		func(g *Grant) { g.LocalTransport = strings.Repeat("a", 64) },
		func(g *Grant) { g.Identity = strings.Repeat("b", 64) },
		func(g *Grant) { g.Role = transport.AuthenticatedPeerConnector },
		func(g *Grant) { g.Alias = "A" },
	} {
		other := g
		change(&other)
		if _, err := readGrantConnectionAt(other, base, now); err == nil {
			t.Fatal("pin/selection changed")
		}
	}
	if _, err := store.RenewTransport(transport.AuthenticatedPeerConnector, g.LocalTransport, now); err != nil {
		t.Fatal(err)
	}
	if _, err := readGrantConnectionAt(g, base, now); !errors.Is(err, ErrGrant) {
		t.Fatal("rotation silently reauthorized boot", err)
	}
}
