package pairing

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestConfirmedCertificatesSupportMutualTLSAndRevocation(t *testing.T) {
	now := time.Now().UTC()
	first, serverCertificate := certifiedIdentity(t, transport.AuthenticatedPeerListener, now)
	second, clientCertificate := certifiedIdentity(t, transport.AuthenticatedPeerConnector, now)
	serverPeer, err := transport.AuthenticatedPeerFromCertificate(serverCertificate.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	clientPeer, err := transport.AuthenticatedPeerFromCertificate(clientCertificate.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	offer, err := CreateOffer(first, serverPeer, now)
	if err != nil {
		t.Fatal(err)
	}
	secondSession, response, err := Accept(offer, second, clientPeer, now)
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := Complete(offer, response, first, now)
	if err != nil {
		t.Fatal(err)
	}
	firstReceipt, err := firstSession.Confirm(firstSession.ConfirmationCode(), now)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, firstStore := testTrustStore(t), testTrustStore(t)
	if _, err := secondSession.FinalizeWithCertificate(firstReceipt, serverCertificate.Certificate[0], secondStore, "first", true, now); !errors.Is(err, ErrLocalConfirmation) {
		t.Fatalf("unconfirmed import: %v", err)
	}
	secondReceipt, err := secondSession.Confirm(secondSession.ConfirmationCode(), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondSession.FinalizeWithCertificate(firstReceipt, clientCertificate.Certificate[0], secondStore, "first", true, now); err == nil {
		t.Fatal("certificate substitution accepted")
	}
	if _, err := secondStore.List(); !errors.Is(err, truststore.ErrStoreNotFound) {
		t.Fatalf("failed pairing wrote trust: %v", err)
	}
	if _, err := secondSession.FinalizeWithCertificate(firstReceipt, serverCertificate.Certificate[0], secondStore, "first", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := firstSession.FinalizeWithCertificate(secondReceipt, clientCertificate.Certificate[0], firstStore, "second", true, now); err != nil {
		t.Fatal(err)
	}
	serverLeaf, err := secondStore.PeerCertificate(serverPeer, now)
	if err != nil {
		t.Fatal(err)
	}
	clientLeaf, err := firstStore.PeerCertificate(clientPeer, now)
	if err != nil {
		t.Fatal(err)
	}
	serverRoots, clientRoots := x509.NewCertPool(), x509.NewCertPool()
	serverRoots.AddCert(serverLeaf)
	clientRoots.AddCert(clientLeaf)
	serverConfig, err := transport.NewServerTLSConfig(serverCertificate, clientRoots)
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err := transport.NewClientTLSConfig(clientCertificate, serverRoots, productidentity.TransportServerName)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverResult := make(chan error, 1)
	go func() {
		actual, err := transport.AuthenticatePeerContext(ctx, tls.Server(left, serverConfig))
		if err == nil && actual != clientPeer {
			err = errors.New("server authenticated wrong peer")
		}
		serverResult <- err
	}()
	actual, clientErr := transport.AuthenticatePeerContext(ctx, tls.Client(right, clientConfig))
	serverErr := <-serverResult
	if clientErr != nil || serverErr != nil || actual != serverPeer {
		t.Fatalf("mTLS client=%v server=%v peer=%+v", clientErr, serverErr, actual)
	}
	if _, err := secondStore.PeerCertificate(serverPeer, serverLeaf.NotAfter); err == nil {
		t.Fatal("expired certificate accepted")
	}
	if _, err := secondStore.Revoke(serverPeer.CertificateSHA256, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := secondStore.PeerCertificate(serverPeer, now.Add(time.Second)); !errors.Is(err, truststore.ErrPeerRevoked) {
		t.Fatalf("revoked certificate read: %v", err)
	}
}

func certifiedIdentity(t *testing.T, role transport.AuthenticatedPeerRole, now time.Time) (*productidentity.Identity, tls.Certificate) {
	t.Helper()
	paths, identity, _, err := productidentity.BootstrapWindows(t.TempDir(), now)
	if err != nil {
		t.Fatal(err)
	}
	store, err := productidentity.New(paths.Identity)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := store.OpenTransport(role, now)
	if err != nil {
		t.Fatal(err)
	}
	return identity, certificate
}
