package truststore

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"os"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const ConnectionSchemaVersion = "krasis.connection-profile/v1"

var (
	ErrAliasAmbiguous          = errors.New("peer alias is ambiguous")
	ErrConnectionNotConfigured = errors.New("connection profile is not configured")
)

// Address is the remote dial address when the peer is LISTENER, and the local
// bind address when the peer is CONNECTOR. Edge is the local exit edge.
type ConnectionSettings struct {
	Address string `json:"address"`
	Edge    string `json:"edge"`
}

type ConnectionProfile struct {
	SchemaVersion string `json:"schemaVersion"`
	Fingerprint   string `json:"fingerprint"`
	ConnectionSettings
}

func (profile ConnectionProfile) Validate() error {
	if profile.SchemaVersion != ConnectionSchemaVersion {
		return errors.New("unsupported connection profile schema")
	}
	if err := validateFingerprint(profile.Fingerprint); err != nil {
		return err
	}
	return profile.ConnectionSettings.Validate()
}

func (settings ConnectionSettings) Validate() error {
	address, err := netip.ParseAddrPort(settings.Address)
	if err != nil || address.Port() == 0 || address.Addr().Zone() != "" {
		return errors.New("connection requires an IP address and nonzero port")
	}
	ip := address.Addr().Unmap()
	if (!ip.IsPrivate() && !ip.IsLoopback()) || ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("connection address must be private or loopback")
	}
	if settings.Edge != "LEFT" && settings.Edge != "RIGHT" {
		return errors.New("connection edge must be LEFT or RIGHT")
	}
	return nil
}

// SetConnection uses the trust-store lock so alias resolution and persistence
// cannot race with a trust change. It never grants trust or inbound permission.
func (store *Store) SetConnection(alias string, settings ConnectionSettings) (ConnectionProfile, error) {
	if store == nil {
		return ConnectionProfile{}, errors.New("trust store is not initialized")
	}
	if err := settings.Validate(); err != nil {
		return ConnectionProfile{}, err
	}
	document, directory, release, err := store.openLocked(false)
	if err != nil {
		return ConnectionProfile{}, err
	}
	defer release()
	peer, err := resolveConnectionPeer(document, alias)
	if err != nil {
		return ConnectionProfile{}, err
	}
	// Refuse to overwrite corrupt existing data as though it were absent.
	if _, err := readConnection(directory, peer.Fingerprint); err != nil && !errors.Is(err, ErrConnectionNotConfigured) {
		return ConnectionProfile{}, err
	}
	profile := ConnectionProfile{SchemaVersion: ConnectionSchemaVersion, Fingerprint: peer.Fingerprint, ConnectionSettings: settings}
	data, err := json.Marshal(profile)
	if err != nil {
		return ConnectionProfile{}, err
	}
	file, name, err := directory.CreateTemporary(".connection-", 0o600)
	if err != nil {
		return ConnectionProfile{}, err
	}
	defer directory.Remove(name)
	defer file.Close()
	if err := peercredentials.ProtectSecurityFileHandle(file); err != nil {
		return ConnectionProfile{}, err
	}
	if _, err := file.Write(data); err != nil {
		return ConnectionProfile{}, err
	}
	if err := file.Sync(); err != nil {
		return ConnectionProfile{}, err
	}
	if err := file.Close(); err != nil {
		return ConnectionProfile{}, err
	}
	if err := directory.Replace(name, connectionFilename(peer.Fingerprint)); err != nil {
		return ConnectionProfile{}, err
	}
	return profile, nil
}

func (store *Store) Connection(alias string) (ConnectionProfile, error) {
	if store == nil {
		return ConnectionProfile{}, errors.New("trust store is not initialized")
	}
	document, directory, release, err := store.openLocked(false)
	if err != nil {
		return ConnectionProfile{}, err
	}
	defer release()
	peer, err := resolveConnectionPeer(document, alias)
	if err != nil {
		return ConnectionProfile{}, err
	}
	return readConnection(directory, peer.Fingerprint)
}

func resolveConnectionPeer(document Document, alias string) (PeerRecord, error) {
	if err := validateAlias(alias); err != nil {
		return PeerRecord{}, err
	}
	var matched *PeerRecord
	for _, peer := range document.Peers {
		if peer.Alias != alias || peer.RevokedAt != nil {
			continue
		}
		if matched != nil {
			return PeerRecord{}, ErrAliasAmbiguous
		}
		matched = &peer
	}
	if matched == nil {
		return PeerRecord{}, ErrPeerNotFound
	}
	return *matched, nil
}

func connectionFilename(fingerprint string) string { return "connection-" + fingerprint + ".json" }

func readConnection(directory *securefs.Directory, fingerprint string) (ConnectionProfile, error) {
	file, err := directory.Open(connectionFilename(fingerprint))
	if errors.Is(err, os.ErrNotExist) {
		return ConnectionProfile{}, ErrConnectionNotConfigured
	}
	if err != nil {
		return ConnectionProfile{}, err
	}
	defer file.Close()
	if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return ConnectionProfile{}, err
	}
	const maxConnectionBytes = 4096
	data, err := io.ReadAll(io.LimitReader(file, maxConnectionBytes+1))
	if err != nil {
		return ConnectionProfile{}, err
	}
	if len(data) > maxConnectionBytes {
		return ConnectionProfile{}, errors.New("connection profile exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var profile ConnectionProfile
	if err := decoder.Decode(&profile); err != nil {
		return ConnectionProfile{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return ConnectionProfile{}, err
	}
	if profile.SchemaVersion != ConnectionSchemaVersion || profile.Fingerprint != fingerprint {
		return ConnectionProfile{}, errors.New("connection profile identity mismatch")
	}
	return profile, profile.Validate()
}
