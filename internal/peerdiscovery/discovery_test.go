package peerdiscovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryOffersRoutingHintWithoutIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := advertiseAt(ctx, 45872, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	found, err := discoverAt(ctx, []*net.UDPAddr{a.conn.LocalAddr().(*net.UDPAddr)})
	if err != nil || len(found) != 1 || found[0] != "127.0.0.1:45872" {
		t.Fatalf("found=%v err=%v", found, err)
	}
	start := time.Now()
	cancel()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("advertisement did not join")
	}
}

func TestDiscoveryRejectsStaleOrPublicHints(t *testing.T) {
	nonce := strings.Repeat("a", 32)
	for _, test := range []struct {
		data, ip string
		valid    bool
	}{
		{fmt.Sprintf(`{"nonce":%q,"port":45872}`, nonce), "192.168.1.2", true},
		{fmt.Sprintf(`{"nonce":%q,"port":45872}`, nonce), "8.8.8.8", false},
		{`{"nonce":"stale","port":45872}`, "192.168.1.2", false},
		{fmt.Sprintf(`{"nonce":%q,"port":65535}`, nonce), "192.168.1.2", false},
		{strings.Repeat("a", 257), "192.168.1.2", false},
	} {
		_, ok := decode([]byte(test.data), nonce, net.ParseIP(test.ip))
		if ok != test.valid {
			t.Fatalf("ip=%s valid=%t want=%t", test.ip, ok, test.valid)
		}
	}
}
