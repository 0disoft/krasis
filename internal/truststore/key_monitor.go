package truststore

import (
	"bytes"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

// NewKeyAnchorMonitor owns one immutable, fully validated certificate for one
// live session. Every check still rereads the current document, certificate and
// their ACLs under the shared lock. Only parsing/signature work on identical
// bytes is reused; a missing, changed or revoked anchor fails immediately.
func (store *Store) NewKeyAnchorMonitor(peer transport.AuthenticatedPeerContext, now time.Time) (func(time.Time, *ReadTrace) error, error) {
	leaf, err := store.PeerKeyAnchor(peer, now)
	if err != nil {
		return nil, err
	}
	approved := bytes.Clone(leaf.Raw)
	return func(now time.Time, trace *ReadTrace) error {
		if now.IsZero() {
			return ErrPeerCertificateUnavailable
		}
		return store.withPeerCertificate(peer, trace, func(current []byte) error {
			trace.enterStep("certificate_match")
			if !bytes.Equal(current, approved) {
				return ErrPeerCertificateUnavailable
			}
			return nil
		})
	}, nil
}
