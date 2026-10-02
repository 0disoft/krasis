package residentagent

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestProductDiscoveryAuthenticatesBeforeSavingEndpoint(t *testing.T) {
	for _, validPeer := range []bool{false, true} {
		t.Run(map[bool]string{false: "different_identity", true: "same_identity"}[validPeer], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			server, client, rogue := newFlowNode(t, ctx), newFlowNode(t, ctx), newFlowNode(t, ctx)
			serverID, err := server.pairing.identities.OpenTransport(transport.AuthenticatedPeerListener, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			clientID, err := client.pairing.identities.OpenTransport(transport.AuthenticatedPeerConnector, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			serverPeer, _ := transport.AuthenticatedPeerFromCertificate(serverID.Leaf)
			clientPeer, _ := transport.AuthenticatedPeerFromCertificate(clientID.Leaf)
			if _, err = server.pairing.trust.ImportWithCertificate(clientPeer, clientID.Certificate[0], "client", true, time.Now()); err != nil {
				t.Fatal(err)
			}
			if _, err = client.pairing.trust.ImportWithCertificate(serverPeer, serverID.Certificate[0], "server", true, time.Now()); err != nil {
				t.Fatal(err)
			}
			if _, err = rogue.pairing.trust.ImportWithCertificate(clientPeer, clientID.Certificate[0], "client", true, time.Now()); err != nil {
				t.Fatal(err)
			}
			for _, node := range []flowNode{server, rogue} {
				if _, err = node.pairing.trust.SetConnection("client", truststore.ConnectionSettings{Address: "127.0.0.1:12345", Edge: "LEFT"}); err != nil {
					t.Fatal(err)
				}
			}
			remote := server
			if !validPeer {
				remote = rogue
			}
			config, err := prepareProductConnection(remote.pairing.identities, remote.pairing.trust, "client")
			if err != nil {
				t.Fatal(err)
			}
			listener, err := tls.Listen("tcp4", "127.0.0.1:0", config.tls)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				raw, err := listener.Accept()
				if err != nil {
					accepted <- nil
					return
				}
				_ = raw.SetDeadline(time.Now().Add(4 * time.Second))
				_ = raw.(*tls.Conn).HandshakeContext(ctx)
				accepted <- raw
			}()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			old := net.JoinHostPort("127.0.0.2", port)
			if _, err = client.pairing.trust.SetConnection("server", truststore.ConnectionSettings{Address: old, Edge: "RIGHT", RearmDistance: 128}); err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareProductConnection(client.pairing.identities, client.pairing.trust, "server")
			if err != nil {
				t.Fatal(err)
			}
			saved := 0
			source := &boundedProductDial{fingerprint: serverPeer.CertificateSHA256, tlsConfig: prepared.tls, load: func() (truststore.ConnectionProfile, error) { return client.pairing.trust.Connection("server") }, discover: func(context.Context) ([]string, error) { return []string{listener.Addr().String()}, nil }, save: func(before truststore.ConnectionProfile, address string) error {
				saved++
				_, err := client.pairing.trust.SetConnectionAddress("server", before.Fingerprint, before.Address, address)
				return err
			}}
			connection, err := source.Next(ctx)
			if connection != nil {
				defer connection.Close()
			}
			raw := <-accepted
			if raw != nil {
				defer raw.Close()
			}
			if validPeer && (err != nil || connection == nil || saved != 1) {
				t.Fatalf("authenticated candidate not used: saved=%d err=%v", saved, err)
			}
			if !validPeer && (err == nil || connection != nil || saved != 0) {
				t.Fatalf("untrusted candidate accepted: saved=%d err=%v", saved, err)
			}
			p, err := client.pairing.trust.Connection("server")
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if validPeer {
				want = listener.Addr().String()
			}
			if p.Address != want || p.Edge != "RIGHT" || p.RearmDistance != 128 || p.Fingerprint != serverPeer.CertificateSHA256 {
				t.Fatalf("wrong persisted profile: %+v", p)
			}
		})
	}
}
