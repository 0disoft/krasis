package loginagent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

const GrantSchema = "krasis.login-grant/v1"

var (
	ErrGrant     = errors.New("login grant is not authorized")
	ErrSuspended = errors.New("login connection is disabled by current user policy")
)

// Grant is an administrator-owned reference to live profile credentials. It
// contains no key, credential copy, executable, command or caller-selected path.
// Decoding this value does not prove the protected provenance of its file.
type Grant struct {
	Schema         string                          `json:"schemaVersion"`
	ID             string                          `json:"id"`
	Owner          string                          `json:"ownerSID"`
	Alias          string                          `json:"alias"`
	Peer           string                          `json:"peerFingerprint"`
	Role           transport.AuthenticatedPeerRole `json:"peerRole"`
	Identity       string                          `json:"identityFingerprint"`
	LocalTransport string                          `json:"localTransportFingerprint"`
	AllowInbound   bool                            `json:"allowInboundControl"`
	CreatedAt      time.Time                       `json:"createdAt"`
	ExpiresAt      time.Time                       `json:"expiresAt"`
}

func (g Grant) Validate(now time.Time) error {
	peer := transport.AuthenticatedPeerContext{CertificateSHA256: g.Peer, Role: g.Role}
	if now.IsZero() || g.Schema != GrantSchema || !accountSID(g.Owner) || peer.Validate() != nil ||
		!pin(g.ID) || !pin(g.Peer) || !pin(g.Identity) || !pin(g.LocalTransport) ||
		g.Alias == "" || strings.TrimSpace(g.Alias) != g.Alias || len(g.Alias) > 256 || strings.ContainsAny(g.Alias, "\r\n\x00") ||
		g.CreatedAt.IsZero() || g.CreatedAt.After(now) || !g.ExpiresAt.After(g.CreatedAt) || !now.Before(g.ExpiresAt) ||
		g.ExpiresAt.Sub(g.CreatedAt) > 30*24*time.Hour {
		return ErrGrant
	}
	return nil
}

func pin(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value || value == strings.Repeat("0", 64) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// Account SIDs exclude SYSTEM, service and group principals. Support ordinary
// local/domain accounts and Entra ID accounts without storing their names.
func accountSID(sid string) bool {
	parts := strings.Split(sid, "-")
	var start int
	switch {
	case strings.HasPrefix(sid, "S-1-5-21-") && len(parts) == 8:
		start = 4
	case strings.HasPrefix(sid, "S-1-12-1-") && len(parts) == 8:
		start = 4
	default:
		return false
	}
	for _, part := range parts[start:] {
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(value, 10) != part {
			return false
		}
	}
	return true
}

func EncodeGrant(g Grant, now time.Time) ([]byte, error) {
	if err := g.Validate(now); err != nil {
		return nil, err
	}
	return json.Marshal(g)
}

func DecodeGrant(data []byte, now time.Time) (Grant, error) {
	if len(data) == 0 || len(data) > 4096 {
		return Grant{}, ErrGrant
	}
	// A flat fixed schema: duplicate fields must not make reviewed bytes and
	// decoded policy disagree. Nested values are rejected before unmarshalling.
	d := json.NewDecoder(bytes.NewReader(data))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return Grant{}, ErrGrant
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return Grant{}, ErrGrant
		}
		seen[key] = true
		value, err := d.Token()
		if err != nil || value == nil {
			return Grant{}, ErrGrant
		}
		if _, nested := value.(json.Delim); nested {
			return Grant{}, ErrGrant
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return Grant{}, ErrGrant
	}
	if _, err := d.Token(); err != io.EOF {
		return Grant{}, ErrGrant
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var g Grant
	if d.Decode(&g) != nil || len(seen) != 11 || g.Validate(now) != nil {
		return Grant{}, ErrGrant
	}
	return g, nil
}

func (g Grant) Selection(now time.Time) (Selection, error) {
	data, err := EncodeGrant(g, now)
	if err != nil {
		return Selection{}, err
	}
	return Selection{Enabled: true, Profile: sha256.Sum256(data)}, nil
}
