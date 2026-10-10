//go:build windows

package loginagent

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Prepared contains private TLS material. It is never a diagnostic or JSON
// response. It is valid only while the live grant and profile are rechecked.
type Prepared struct {
	Profile   truststore.ConnectionProfile
	Peer      truststore.PeerRecord
	Identity  productidentity.PublicIdentity
	Local     tls.Certificate
	Remote    *x509.Certificate
	ExpiresAt time.Time
}

// ResolveOwnerAppData uses the administrator-controlled ProfileList, never a
// path supplied in the grant/IPC or another user's environment. Redirected local
// application data must be explicitly supported before provisioning it.
func ResolveOwnerAppData(owner string) (string, error) {
	if !accountSID(owner) {
		return "", ErrGrant
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+owner, registry.QUERY_VALUE)
	if err != nil {
		return "", ErrGrant
	}
	defer key.Close()
	path, kind, err := key.GetStringValue("ProfileImagePath")
	if err != nil || kind != registry.SZ && kind != registry.EXPAND_SZ {
		return "", ErrGrant
	}
	if kind == registry.EXPAND_SZ {
		path, err = registry.ExpandString(path)
		if err != nil {
			return "", ErrGrant
		}
	}
	if len(path) < 3 || path[1] != ':' || path[2] != '\\' || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, '\x00') {
		return "", ErrGrant
	}
	return filepath.Join(path, "AppData", "Local"), nil
}

func readLive(owner, base, alias, fingerprint string, now time.Time) (Prepared, error) {
	settings, resume, err := appsettings.ReadOwnerStartup(base, owner)
	if err != nil {
		return Prepared{}, ErrGrant
	}
	if !settings.StartAtLogin || resume.ManualStop || resume.Alias != alias || resume.Fingerprint != fingerprint {
		return Prepared{}, ErrSuspended
	}
	peer, profile, remote, err := truststore.ReadOwnerConnection(base, owner, alias, fingerprint, now)
	if err != nil {
		return Prepared{}, ErrGrant
	}
	role := transport.AuthenticatedPeerListener
	if peer.Role == transport.AuthenticatedPeerListener {
		role = transport.AuthenticatedPeerConnector
	}
	identity, local, err := productidentity.ReadOwnerTransport(base, owner, role, now)
	if err != nil {
		return Prepared{}, ErrGrant
	}
	expires := remote.NotAfter
	if local.Leaf.NotAfter.Before(expires) {
		expires = local.Leaf.NotAfter
	}
	return Prepared{Profile: profile, Peer: peer, Identity: identity, Local: local, Remote: remote, ExpiresAt: expires}, nil
}

// PrepareCurrentGrant selects only the exact remembered peer of the account
// running the elevated configuration command. It never makes a new credential.
func PrepareCurrentGrant(alias string, now time.Time) (Grant, error) {
	var token windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token) != nil {
		return Grant{}, ErrGrant
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || !token.IsElevated() {
		return Grant{}, ErrGrant
	}
	owner := user.User.Sid.String()
	base, err := ResolveOwnerAppData(owner)
	if err != nil {
		return Grant{}, ErrGrant
	}
	currentBase, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil || !strings.EqualFold(base, currentBase) {
		return Grant{}, ErrGrant
	}
	_, resume, err := appsettings.ReadOwnerStartup(base, owner)
	if err != nil || resume.ManualStop || resume.Alias != alias || resume.Fingerprint == "" {
		return Grant{}, ErrSuspended
	}
	live, err := readLive(owner, base, alias, resume.Fingerprint, now)
	if err != nil {
		return Grant{}, err
	}
	local, err := transport.AuthenticatedPeerFromCertificate(live.Local.Leaf)
	if err != nil {
		return Grant{}, ErrGrant
	}
	var id [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Grant{}, ErrGrant
	}
	g := Grant{Schema: GrantSchema, ID: hex.EncodeToString(id[:]), Owner: owner, Alias: alias, Peer: resume.Fingerprint,
		Role: live.Peer.Role, Identity: live.Identity.Fingerprint, LocalTransport: local.CertificateSHA256,
		AllowInbound: live.Peer.AllowInboundControl, CreatedAt: now.UTC(), ExpiresAt: live.ExpiresAt}
	return g, g.Validate(now)
}

// ReadGrantConnection reloads live revocation/manual-stop policy and both exact
// credential pins. Version updates do not require reprovisioning this grant.
func ReadGrantConnection(g Grant, now time.Time) (Prepared, error) {
	if g.Validate(now) != nil {
		return Prepared{}, ErrGrant
	}
	base, err := ResolveOwnerAppData(g.Owner)
	if err != nil {
		return Prepared{}, ErrGrant
	}
	return readGrantConnectionAt(g, base, now)
}

func readGrantConnectionAt(g Grant, base string, now time.Time) (Prepared, error) {
	if g.Validate(now) != nil {
		return Prepared{}, ErrGrant
	}
	live, err := readLive(g.Owner, base, g.Alias, g.Peer, now)
	if err != nil {
		return Prepared{}, err
	}
	actual, err := transport.AuthenticatedPeerFromCertificate(live.Local.Leaf)
	if err != nil || live.Peer.Role != g.Role || live.Identity.Fingerprint != g.Identity || actual.CertificateSHA256 != g.LocalTransport || live.ExpiresAt.Before(g.ExpiresAt) {
		return Prepared{}, ErrGrant
	}
	live.Peer.AllowInboundControl = g.AllowInbound && live.Peer.AllowInboundControl
	live.ExpiresAt = g.ExpiresAt
	// Recheck a manual stop that raced the credential/trust reads.
	settings, resume, err := appsettings.ReadOwnerStartup(base, g.Owner)
	if err != nil {
		return Prepared{}, ErrGrant
	}
	if !settings.StartAtLogin || resume.ManualStop || resume.Alias != g.Alias || resume.Fingerprint != g.Peer {
		return Prepared{}, ErrSuspended
	}
	return live, nil
}
