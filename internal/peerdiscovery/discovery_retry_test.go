package peerdiscovery

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryRecoversWhenFirstProbeIsLost(t *testing.T) {
	responder, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		var buffer [256]byte
		var firstNonce string
		for {
			n, sender, err := responder.ReadFromUDP(buffer[:])
			if err != nil {
				return
			}
			nonce := strings.TrimPrefix(string(buffer[:n]), prefix)
			if probes.Add(1) == 1 {
				firstNonce = nonce
				continue // Simulate losing the first request or its reply.
			}
			if nonce != firstNonce {
				t.Error("retry changed discovery nonce")
			}
			data, _ := json.Marshal(announcement{Nonce: nonce, Port: 45872})
			_, _ = responder.WriteToUDP(data, sender)
		}
	}()
	defer func() { _ = responder.Close(); <-done }()
	found, err := discoverAt(context.Background(), []*net.UDPAddr{responder.LocalAddr().(*net.UDPAddr)})
	if err != nil || len(found) != 1 || found[0] != "127.0.0.1:45872" {
		t.Fatalf("first probe loss prevented discovery: probes=%d, found=%v, err=%v", probes.Load(), found, err)
	}
	if count := probes.Load(); count < 2 || count > 6 {
		t.Fatalf("probe count exceeded the original discovery budget: %d", count)
	}
}

func TestDiscoveryCancellationInterruptsRetryWait(t *testing.T) {
	responder, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer responder.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := discoverAt(ctx, []*net.UDPAddr{responder.LocalAddr().(*net.UDPAddr)})
		done <- err
	}()
	if err := responder.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var buffer [256]byte
	if _, _, err := responder.ReadFromUDP(buffer[:]); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("cancellation returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery kept waiting after cancellation")
	}
}
