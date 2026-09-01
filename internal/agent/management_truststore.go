package agent

import (
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

type TrustStoreRevoker struct {
	Store *truststore.Store
}

func (revoker TrustStoreRevoker) RevokeTrust(fingerprint string, now time.Time) error {
	if revoker.Store == nil {
		return errors.New("trust store is required")
	}
	_, err := revoker.Store.Revoke(fingerprint, now)
	if errors.Is(err, truststore.ErrPeerRevoked) {
		return nil
	}
	return err
}
