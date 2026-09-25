package residentagent

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestProductDialReloadsEndpointWithoutChangingPeer(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	accepted := make(chan string, 2)
	for _, listener := range []struct {
		name string
		net.Listener
	}{{"first", first}, {"second", second}} {
		go func() {
			connection, acceptErr := listener.Accept()
			if acceptErr == nil {
				accepted <- listener.name
				_ = connection.Close()
			}
		}()
	}
	profile := truststore.ConnectionProfile{Fingerprint: "trusted"}
	profile.Address = first.Addr().String()
	source := &boundedProductDial{
		load:        func() (truststore.ConnectionProfile, error) { return profile, nil },
		fingerprint: "trusted",
		tlsConfig:   &tls.Config{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, target := range []struct {
		name    string
		address string
	}{{"first", first.Addr().String()}, {"second", second.Addr().String()}} {
		profile.Address = target.address
		if connection, dialErr := source.Next(ctx); connection != nil || dialErr == nil {
			t.Fatalf("dial to %s unexpectedly authenticated: connection=%v error=%v", target.name, connection, dialErr)
		}
		select {
		case name := <-accepted:
			if name != target.name {
				t.Fatalf("dial reached %s, want %s", name, target.name)
			}
		case <-ctx.Done():
			t.Fatalf("dial to %s did not reach endpoint: %v", target.name, ctx.Err())
		}
	}
	profile.Fingerprint = "different"
	if connection, dialErr := source.Next(ctx); connection != nil || dialErr == nil {
		t.Fatalf("changed identity unexpectedly dialed: connection=%v error=%v", connection, dialErr)
	}
	select {
	case name := <-accepted:
		t.Fatalf("changed identity reached %s", name)
	default:
	}
}
