package truststore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/0disoft/krasis/internal/transport"
)

const SchemaVersion = "krasis.trust-store/v1"

type Document struct {
	SchemaVersion string       `json:"schemaVersion"`
	Revision      uint64       `json:"revision"`
	UpdatedAt     time.Time    `json:"updatedAt"`
	Peers         []PeerRecord `json:"peers"`
}

type PeerRecord struct {
	Fingerprint         string                          `json:"fingerprint"`
	Role                transport.AuthenticatedPeerRole `json:"role"`
	Alias               string                          `json:"alias"`
	AllowInboundControl bool                            `json:"allowInboundControl"`
	CreatedAt           time.Time                       `json:"createdAt"`
	RevokedAt           *time.Time                      `json:"revokedAt,omitempty"`
}

func (document Document) Validate() error {
	if document.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported trust store schema %q", document.SchemaVersion)
	}
	if document.Revision == 0 || document.UpdatedAt.IsZero() {
		return errors.New("trust store revision and updated time are required")
	}
	if document.Peers == nil {
		return errors.New("trust store peers must be an array")
	}
	seen := make(map[string]struct{}, len(document.Peers))
	for _, record := range document.Peers {
		if err := record.Validate(); err != nil {
			return err
		}
		if _, exists := seen[record.Fingerprint]; exists {
			return errors.New("trust store contains a duplicate fingerprint")
		}
		seen[record.Fingerprint] = struct{}{}
		if record.CreatedAt.After(document.UpdatedAt) {
			return errors.New("trusted peer creation time is after store update time")
		}
		if record.RevokedAt != nil && record.RevokedAt.After(document.UpdatedAt) {
			return errors.New("trusted peer revocation time is after store update time")
		}
	}
	return nil
}

func (record PeerRecord) Validate() error {
	if err := validateFingerprint(record.Fingerprint); err != nil {
		return err
	}
	peer := transport.AuthenticatedPeerContext{CertificateSHA256: record.Fingerprint, Role: record.Role}
	if err := peer.Validate(); err != nil {
		return err
	}
	if err := validateAlias(record.Alias); err != nil {
		return err
	}
	if record.CreatedAt.IsZero() {
		return errors.New("trusted peer creation time is required")
	}
	if record.RevokedAt != nil && (record.RevokedAt.IsZero() || record.RevokedAt.Before(record.CreatedAt)) {
		return errors.New("trusted peer revocation time is invalid")
	}
	return nil
}

func validateFingerprint(fingerprint string) error {
	if len(fingerprint) != 64 || strings.ToLower(fingerprint) != fingerprint {
		return errors.New("trusted peer fingerprint is invalid")
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return errors.New("trusted peer fingerprint is invalid")
	}
	return nil
}

func validateAlias(alias string) error {
	if alias == "" || strings.TrimSpace(alias) != alias || !utf8.ValidString(alias) || utf8.RuneCountInString(alias) > 64 {
		return errors.New("trusted peer alias must contain 1 to 64 characters without surrounding whitespace")
	}
	for _, value := range alias {
		if unicode.IsControl(value) {
			return errors.New("trusted peer alias must not contain control characters")
		}
	}
	return nil
}

func sortPeers(peers []PeerRecord) {
	sort.Slice(peers, func(left, right int) bool {
		return peers[left].Fingerprint < peers[right].Fingerprint
	})
}
