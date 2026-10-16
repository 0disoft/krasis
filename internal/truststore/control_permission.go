package truststore

import (
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

// InboundControlAllowed checks the current permission, role, revocation and
// exact key anchor under one shared-lock snapshot. Nothing is cached between
// requests; the directory, document and certificate ACL checks remain fresh.
func (store *Store) InboundControlAllowed(peer transport.AuthenticatedPeerContext, now time.Time) (bool, error) {
	allowed := false
	err := store.withPeerRecordCertificate(peer, nil, func(record PeerRecord, der []byte) error {
		if _, err := validatePeerKeyAnchor(peer, der, now); err != nil {
			return err
		}
		allowed = record.AllowInboundControl
		return nil
	})
	return allowed, err
}
