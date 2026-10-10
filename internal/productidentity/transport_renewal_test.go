package productidentity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func TestAutomaticLeafRenewalKeepsPairedKeyAndNeverChangesStore(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	created := now.Add(-90 * 24 * time.Hour)
	identity, _, err := store.OpenOrCreate(created)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := store.OpenTransport(transport.AuthenticatedPeerListener, created)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{created.Add(time.Hour), now, now.Add(120 * 24 * time.Hour)} {
		renewed, err := RenewableTransport(anchor, at)
		if err != nil {
			t.Fatal(err)
		}
		if at.Before(renewed.Leaf.NotBefore) || !at.Before(renewed.Leaf.NotAfter) ||
			!bytes.Equal(anchor.Leaf.RawSubjectPublicKeyInfo, renewed.Leaf.RawSubjectPublicKeyInfo) {
			t.Fatal("invalid renewal")
		}
		state := tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{renewed.Leaf}}
		got, err := transport.AuthenticateProductKey(state, anchor.Leaf, at)
		want, _ := transport.AuthenticatedPeerFromCertificate(anchor.Leaf)
		if err != nil || got != want {
			t.Fatalf("lost original principal: %v", err)
		}
	}
	stored, err := store.OpenTransportAnchor(transport.AuthenticatedPeerListener)
	if err != nil || !bytes.Equal(stored.Certificate[0], anchor.Certificate[0]) {
		t.Fatal("renewal modified stored anchor")
	}
	current, err := store.Load()
	if err != nil || current.Public() != identity.Public() {
		t.Fatal("device identity changed")
	}
	if _, err := store.Rotate(now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenTransportAnchor(transport.AuthenticatedPeerListener); err == nil {
		t.Fatal("identity reset kept old role authority")
	}
}

func TestRenewedMutualTLSKeepsPairingAndRejectsUnpairedKey(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	created := now.Add(-60 * 24 * time.Hour)
	get := func(role transport.AuthenticatedPeerRole) tls.Certificate {
		s := testStore(t)
		if _, _, err := s.OpenOrCreate(created); err != nil {
			t.Fatal(err)
		}
		c, err := s.OpenTransport(role, created)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	server := get(transport.AuthenticatedPeerListener)
	client := get(transport.AuthenticatedPeerConnector)
	unpaired := get(transport.AuthenticatedPeerConnector)
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint("unpaired=", bad), func(t *testing.T) {
			chosen := client
			if bad {
				chosen = unpaired
			}
			cfgS, err := transport.NewProductKeyTLSConfig(func() (tls.Certificate, error) { return RenewableTransport(server, time.Now()) }, client.Leaf, true, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			cfgC, err := transport.NewProductKeyTLSConfig(func() (tls.Certificate, error) { return RenewableTransport(chosen, time.Now()) }, server.Leaf, false, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				c, e := transport.NewPeerConnection(ctx, tls.Server(a, cfgS), client.Leaf)
				if e == nil {
					actual, _ := c.AuthenticatedPeer()
					want, _ := transport.AuthenticatedPeerFromCertificate(client.Leaf)
					if actual != want {
						e = errors.New("changed principal")
					}
				}
				done <- e
				if e != nil {
					a.Close()
				}
			}()
			c, ce := transport.NewPeerConnection(ctx, tls.Client(b, cfgC), server.Leaf)
			se := <-done
			if bad {
				if se == nil {
					t.Fatal("unpaired role key accepted")
				}
				return
			}
			if se != nil || ce != nil {
				t.Fatalf("renewed handshake: server=%v client=%v", se, ce)
			}
			actual, _ := c.AuthenticatedPeer()
			want, _ := transport.AuthenticatedPeerFromCertificate(server.Leaf)
			if actual != want {
				t.Fatal("server pairing principal changed")
			}
			state := tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{server.Leaf}}
			if _, err := transport.AuthenticateProductKey(state, server.Leaf, now); err == nil {
				t.Fatal("expired on-wire leaf accepted")
			}
		})
	}
}
