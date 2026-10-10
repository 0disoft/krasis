package productidentity

import (
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func TestConcurrentRenewalReadersKeepKeyReplacementExcluded(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	if _, _, err := store.OpenOrCreate(now); err != nil {
		t.Fatal(err)
	}
	role := transport.AuthenticatedPeerListener
	c, err := store.OpenTransport(role, now)
	if err != nil {
		t.Fatal(err)
	}
	pin, _ := transport.AuthenticatedPeerFromCertificate(c.Leaf)
	_, release, err := store.openTransportReadLocked()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := store.OpenTransportAnchor(role); err != nil {
		t.Fatal("TLS reader blocked input-start reader", err)
	}
	if _, err := store.InspectRenewableTransport(role, now); err != nil {
		t.Fatal("CLI reader blocked input-start reader", err)
	}
	if _, err := store.RenewTransport(role, pin.CertificateSHA256, now); !errors.Is(err, ErrStoreBusy) {
		t.Fatal("writer did not exclude reader", err)
	}
}
