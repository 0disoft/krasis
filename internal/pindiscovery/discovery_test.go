package pindiscovery

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryNeverTreatsRepliesAsAuthority(t *testing.T) {
	nonce := strings.Repeat("a", 32)
	value := announcement{nonce, strings.Repeat("b", 32), 45871}
	data, _ := json.Marshal(value)
	got, ok := decode(data, nonce, net.ParseIP("192.168.1.7"))
	if !ok || got.Address != "192.168.1.7:45871" {
		t.Fatal("valid hint rejected")
	}
	for _, test := range []struct{ nonce, ip string }{{"wrong", "192.168.1.7"}, {nonce, "8.8.8.8"}} {
		if _, ok := decode(data, test.nonce, net.ParseIP(test.ip)); ok {
			t.Fatal("unrelated discovery reply accepted")
		}
	}
	if _, ok := decode([]byte(strings.Repeat("x", 513)), nonce, net.ParseIP("192.168.1.7")); ok {
		t.Fatal("oversize reply accepted")
	}
}

func TestInvitationRespondsAndClosesSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closer, err := advertiseAt(ctx, strings.Repeat("b", 32), 45871, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	server := closer.(*net.UDPConn)
	client, err := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetDeadline(time.Now().Add(time.Second))
	nonce := strings.Repeat("a", 32)
	if _, err = client.Write([]byte(prefix + nonce)); err != nil {
		t.Fatal(err)
	}
	var buf [512]byte
	n, err := client.Read(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decode(buf[:n], nonce, net.IPv4(127, 0, 0, 1)); !ok {
		t.Fatal("invalid invitation response")
	}
	cancel()
	closer.Close()
	if _, err := server.WriteToUDP([]byte("closed"), client.LocalAddr().(*net.UDPAddr)); err == nil {
		t.Fatal("invitation socket survived close")
	}
}
