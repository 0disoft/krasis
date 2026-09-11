package pairing

import (
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
	return store.ImportWithCertificate(session.remote.TransportPeer, certificateDER, alias, allowInboundControl, now.UTC())
}
