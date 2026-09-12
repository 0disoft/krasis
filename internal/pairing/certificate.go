package pairing

import (
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

// FinalizeWithCertificate binds the leaf received by the transport adapter to
// the exact peer fingerprint and role in the mutually confirmed transcript.
// Certificate bytes are not added to offers, responses, receipts, or diagnostics.
func (session *Session) FinalizeWithCertificate(remote Receipt, certificateDER []byte, store *truststore.Store, alias string, allowInboundControl bool, now time.Time) (truststore.PeerRecord, error) {
	if err := session.authorizeFinalize(remote, store, now); err != nil {
		return truststore.PeerRecord{}, err
	}
	record, err := store.ImportWithCertificate(session.remote.TransportPeer, certificateDER, alias, allowInboundControl, now.UTC())
	if errors.Is(err, truststore.ErrPeerExists) {
		// A peer whose certificate did not rotate can retain its exact existing
		// grant. Recheck under the store lock; never infer consent from existence.
		return store.ReconfirmWithCertificate(session.remote.TransportPeer, certificateDER, alias, allowInboundControl, now.UTC())
	}
	return record, err
}

// FinalizeReplacementWithCertificate verifies the new peer before replacing the
// caller-selected old fingerprint. The caller owns selection/approval of the old
// peer and must stop its active session before invoking this persistence path.
func (session *Session) FinalizeReplacementWithCertificate(remote Receipt, certificateDER []byte, store *truststore.Store, replacedFingerprint, alias string, allowInboundControl bool, now time.Time) (truststore.PeerRecord, truststore.PeerRecord, error) {
	if err := session.authorizeFinalize(remote, store, now); err != nil {
		return truststore.PeerRecord{}, truststore.PeerRecord{}, err
	}
	return store.ReplaceWithCertificate(replacedFingerprint, session.remote.TransportPeer, certificateDER, alias, allowInboundControl, now.UTC())
}
